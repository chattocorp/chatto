//go:build bootstrap || test_endpoints

package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

func (a *API) operatorSeedHandlers(options []connect.HandlerOption) []Handler {
	path, handler := operatorv1connect.NewOperatorSeedServiceHandler(&operatorSeedService{api: a}, options...)
	return []Handler{{ServicePath: path, Handler: handler, AuthPolicy: AuthPolicyPublic}}
}

type operatorSeedService struct{ api *API }

func (s *operatorSeedService) SeedData(ctx context.Context, req *connect.Request[operatorv1.SeedDataRequest]) (*connect.Response[operatorv1.SeedDataResponse], error) {
	r, err := s.api.core.SeedData(ctx, core.SeedOptions{Seed: req.Msg.Seed,
		Users: int(req.Msg.Users), Rooms: int(req.Msg.Rooms), Messages: int(req.Msg.Messages), ThreadReplies: int(req.Msg.ThreadReplies)})
	if err != nil {
		return nil, connectError(err)
	}
	response := &operatorv1.SeedDataResponse{Version: r.Version, Seed: r.Seed}
	for _, user := range r.Users {
		response.Users = append(response.Users, &operatorv1.SeedUser{Id: user.ID, Login: user.Login, DisplayName: user.DisplayName})
	}
	for _, room := range r.Rooms {
		response.Rooms = append(response.Rooms, &operatorv1.SeedRoom{Id: room.ID, Name: room.Name})
	}
	for _, message := range r.Messages {
		response.Messages = append(response.Messages, &operatorv1.SeedMessage{Id: message.ID, RoomId: message.RoomID, AuthorId: message.AuthorID, Body: message.Body, ThreadRootId: message.ThreadRootID})
	}
	return connect.NewResponse(response), nil
}
