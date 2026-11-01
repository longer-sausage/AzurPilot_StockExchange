package exchange

func publicOrder(o *Order) map[string]any {
	if o == nil {
		return nil
	}
	return map[string]any{"id": o.ID, "kind": o.Kind, "timeInForce": o.TimeInForce, "expiresAt": o.ExpiresAt, "triggered": o.Triggered, "stockId": o.StockID, "clientId": o.ClientID, "side": o.Side, "quantity": o.Quantity, "remaining": o.Remaining, "filled": o.Filled, "price": o.Price, "averagePrice": o.AveragePrice, "notional": o.Notional, "tax": o.Tax, "commission": o.Commission, "stamp": o.Stamp, "levy": o.Levy, "taxRatePPM": o.TaxRatePPM, "status": o.Status, "reserved": o.Reserved, "createdAt": o.CreatedAt, "updatedAt": o.UpdatedAt, "reason": o.Reason, "leverage": o.Leverage}
}
