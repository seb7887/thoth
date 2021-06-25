package auth

import (
	"context"
	"fmt"

	"github.com/seb7887/thoth/config"
	"github.com/seb7887/thoth/heimdallr"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Auth interface {
	CheckConnect(username, password string) bool
}

type AuthClient struct{}

func NewAuth() (Auth, error) {
	return &AuthClient{}, nil
}

func (a *AuthClient) CheckConnect(username, password string) bool {
	config := config.GetConfig()
	heimdallrAddr := fmt.Sprintf("%s:%d", config.HeimdallrHost, config.HeimdallrPort)
	conn, err := grpc.Dial(heimdallrAddr, grpc.WithInsecure())
	if err != nil {
		log.Errorf("failed to connect to auth service %s", err.Error())
		return false
	}
	defer conn.Close()

	client := heimdallr.NewHeimdallrServiceClient(conn)
	md := metadata.Pairs("authorization", config.HeimdallrAPIKey)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	req := heimdallr.AuthRequest{
		ClientId: username,
		JwtToken: password,
	}
	res, err := client.Authenticate(ctx, &req)
	if err != nil {
		log.Errorf("failed to perform HTTP/2 request to auth service %s", err.Error())
		return false
	}

	result := res.GetSuccess()

	return result
}
