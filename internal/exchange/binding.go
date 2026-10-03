package exchange

func (e *Engine) quoteFresh(p *Player, now int64) bool {
	return p.Quote.Price > 0 && now-p.Quote.ObservedAt <= e.state.Settings.Active.QuoteTTLSeconds
}
func (e *Engine) checkBinding(id int64, b InstanceBinding) error {
	if other := e.instances[b.Key]; other != 0 && other != id {
		return fail("INSTANCE_TAKEN", "当前 AzurPilot 实例已永久绑定其他账户")
	}
	if other := e.instanceIDs[b.InstanceID]; other != 0 && other != id {
		return fail("INSTANCE_TAKEN", "当前实例标识已绑定其他账户")
	}
	return nil
}
func (e *Engine) BindInstance(id int64, b *InstanceBinding) (*Player, error) {
	var result *Player
	err := e.transaction(func() error {
		if err := e.advance(e.clock()); err != nil {
			return err
		}
		p := e.players[id]
		if p == nil || p.Disabled {
			return fail("UNAUTHORIZED", "账户不可用")
		}
		if p.Binding != nil {
			if p.Binding.Key != b.Key || p.Binding.InstanceID != b.InstanceID {
				return fail("INSTANCE_MISMATCH", "此账户已永久绑定其他 AzurPilot 实例")
			}
			result = clonePlayer(p)
			return nil
		}
		if err := e.checkBinding(id, *b); err != nil {
			return err
		}
		p = e.touch(id)
		binding := *b
		p.Binding = &binding
		result = clonePlayer(p)
		return nil
	})
	return result, err
}
func (e *Engine) SessionBinding(id int64) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if p := e.players[id]; p != nil && p.Binding != nil {
		return p.Binding.Key
	}
	return ""
}
