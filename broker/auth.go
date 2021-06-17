package broker

func (b *Broker) AuthenticateClient(username, password string) bool {
	if b.auth != nil {
		return b.auth.CheckConnect(username, password)
	}
	return true
}
