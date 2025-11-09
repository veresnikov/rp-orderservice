package service

import (
	"context"
	"errors"

	"gitea.xscloud.ru/xscloud/golib/pkg/application/outbox"
	"github.com/google/uuid"

	"order/pkg/domain/model"
	"order/pkg/domain/service"
)

type ProductProvider interface {
	ActualPrice(ctx context.Context, productID uuid.UUID) (float64, error)
}

type OrderService interface {
	AddProductToOrder(ctx context.Context, customerID uuid.UUID, itemID uuid.UUID) (uuid.UUID, error)
}

func NewOrderService() OrderService {
	return &orderService{}
}

type orderService struct {
	uow             UnitOfWork
	luow            LockableUnitOfWork
	eventDispatcher outbox.EventDispatcher[service.Event]
	productProvider ProductProvider
}

func (o orderService) AddProductToOrder(ctx context.Context, customerID uuid.UUID, productID uuid.UUID) (uuid.UUID, error) {
	var orderID uuid.UUID
	err := o.luow.Execute(ctx, "customer_"+customerID.String(), func(provider RepositoryProvider) error {
		status := model.Open
		order, err := provider.OrderRepository(ctx).Find(model.FindSpec{
			CustomerID: &customerID,
			Status:     &status,
		})
		if err == nil {
			orderID = order.ID
			return nil
		}
		if err != nil && !errors.Is(err, model.ErrOrderNotFound) {
			return err
		}

		domainService := o.domainService(ctx, provider.OrderRepository(ctx))
		orderID, err = domainService.CreateOrder(customerID)
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}

	price, err := o.productProvider.ActualPrice(ctx, productID)
	if err != nil {
		return uuid.Nil, err
	}

	var itemID uuid.UUID
	return itemID, o.luow.Execute(ctx, "order_"+orderID.String(), func(provider RepositoryProvider) error {
		domainService := o.domainService(ctx, provider.OrderRepository(ctx))
		itemID, err = domainService.AddItem(orderID, productID, price)
		return err
	})
}

func (o orderService) domainService(ctx context.Context, repo model.OrderRepository) service.Order {
	return service.NewOrderService(
		repo,
		&domainEventDispatcher{
			ctx:             ctx,
			eventDispatcher: o.eventDispatcher,
		},
	)
}

type domainEventDispatcher struct {
	ctx             context.Context
	eventDispatcher outbox.EventDispatcher[service.Event]
}

func (d domainEventDispatcher) Dispatch(event service.Event) error {
	return d.eventDispatcher.Dispatch(d.ctx, event)
}
