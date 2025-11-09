package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitea.xscloud.ru/xscloud/golib/pkg/infrastructure/mysql"
	"github.com/google/uuid"

	"order/pkg/domain/model"
)

func NewOrderRepository(ctx context.Context, client mysql.ClientContext) model.OrderRepository {
	return &orderRepository{
		ctx:    ctx,
		client: client,
	}
}

type orderRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func (o orderRepository) NextID() (uuid.UUID, error) {
	return uuid.NewV7()
}

func (o orderRepository) Store(order *model.Order) error {
	err := o.storeOrder(order)
	if err != nil {
		return err
	}

	err = o.storeItems(order.ID, order.Items)
	if err != nil {
		return err
	}

	return nil
}

func (o orderRepository) storeOrder(order *model.Order) error {
	const storeOrder = `
		INSERT INTO order (
			order_id,
		    customer_id,
		    status,
		    created_at,
		    updated_at,
		    deleted_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status=VALUES(status),
			updated_at=VALUES(updated_at),
			deleted_at=VALUES(deleted_at)
	`

	_, err := o.client.ExecContext(
		o.ctx, storeOrder,
		order.ID, order.CustomerID, order.Status, order.CreatedAt, order.UpdatedAt, order.DeletedAt,
	)
	return err
}

func (o orderRepository) storeItems(orderID uuid.UUID, items []model.Item) error {
	const deleteItems = `DELETE FROM item WHERE order_id = ?`
	_, err := o.client.ExecContext(o.ctx, deleteItems, orderID)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	const storeItems = `INSERT INTO item (order_id, product_id, price) VALUES (?, ?, ?)`
	for _, item := range items {
		_, err = o.client.ExecContext(o.ctx, storeItems, orderID, item.ProductID, item.Price)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o orderRepository) Find(spec model.FindSpec) (*model.Order, error) {
	const findItems = `
		SELECT
			order_id,
			customer_id,
			status,
			created_at,
			updated_at,
			deleted_at
		FROM order
		WHERE %s
	`
	whereQuery, args := o.buildWhereConditions(spec)

	var order sqlxOrder
	err := o.client.GetContext(o.ctx, &order, fmt.Sprintf(findItems, whereQuery), args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderNotFound
		}
		return nil, err
	}

	// TODO load items

	return &model.Order{
		ID:         order.ID,
		CustomerID: order.CustomerID,
		Status:     model.OrderStatus(order.Status),
		Items:      nil,
		CreatedAt:  order.CreatedAt,
		UpdatedAt:  order.UpdatedAt,
		DeletedAt:  order.DeletedAt,
	}, nil
}

func (o orderRepository) Delete(id uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

func (o orderRepository) buildWhereConditions(spec model.FindSpec) (query string, args []interface{}) {
	var parts []string
	if spec.OrderID != nil {
		parts = append(parts, "order_id = ?")
		args = append(args, *spec.OrderID)
	}
	if spec.CustomerID != nil {
		parts = append(parts, "customer_id = ?")
		args = append(args, *spec.CustomerID)
	}
	if spec.Status != nil {
		parts = append(parts, "status = ?")
		args = append(args, *spec.Status)
	}
	if !spec.IncludeDeleted {
		parts = append(parts, "deleted_at is null")
	}
	return strings.Join(parts, " AND "), args
}

type sqlxOrder struct {
	ID         uuid.UUID  `db:"order_id"`
	CustomerID uuid.UUID  `db:"customer_id"`
	Status     int        `db:"status"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
	DeletedAt  *time.Time `db:"deleted_at"`
}

type sqlxItem struct {
	OrderID   uuid.UUID `db:"order_id"`
	ProductID uuid.UUID `db:"product_id"`
	Price     float64   `db:"price"`
}
