package auth

type Auth interface {
	CheckConnect(username, password string) bool
}

type AuthClient struct{}

func NewAuth() (Auth, error) {
	return &AuthClient{}, nil
}

func (a *AuthClient) CheckConnect(username, password string) bool {
	return username == "testUser" && password == "testPwd"
}
