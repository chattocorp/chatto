package connectapi

import (
	"context"
	"net"
	"net/url"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	personaldataapiv1 "hmans.de/chatto/internal/pb/chatto/personaldata/api/v1"
	personaldatav1 "hmans.de/chatto/internal/pb/chatto/personaldata/v1"
)

type personalDataService struct {
	api *API
}

func (s *personalDataService) GetPreferences(ctx context.Context, _ *connect.Request[personaldataapiv1.GetPreferencesRequest]) (*connect.Response[personaldataapiv1.GetPreferencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	preferences, err := s.api.core.PersonalData.GetPreferences(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.GetPreferencesResponse{Preferences: personalPreferencesToAPI(preferences)}), nil
}

func (s *personalDataService) UpdatePreferences(ctx context.Context, req *connect.Request[personaldataapiv1.UpdatePreferencesRequest]) (*connect.Response[personaldataapiv1.UpdatePreferencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	paths, err := preferenceUpdatePaths(req.Msg.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, err
	}
	input := req.Msg.GetPreferences()
	preferences, err := s.api.core.PersonalData.UpdatePreferences(ctx, caller.UserID, func(current *personaldatav1.Preferences) error {
		for _, path := range paths {
			switch path {
			case "locale":
				current.Locale = cloneString(input.Locale)
			case "timezone":
				current.Timezone = cloneString(input.Timezone)
			case "time_format":
				if input.TimeFormat == nil {
					current.TimeFormat = nil
				} else {
					value := apiPersonalTimeFormatToStored(input.GetTimeFormat())
					current.TimeFormat = &value
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.UpdatePreferencesResponse{Preferences: personalPreferencesToAPI(preferences)}), nil
}

func (s *personalDataService) ListKnownServers(ctx context.Context, _ *connect.Request[personaldataapiv1.ListKnownServersRequest]) (*connect.Response[personaldataapiv1.ListKnownServersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	directory, err := s.api.core.PersonalData.ListServers(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	servers := make([]*personaldataapiv1.KnownServer, 0, len(directory.GetServers()))
	for _, server := range directory.GetServers() {
		servers = append(servers, personalServerToAPI(server))
	}
	return connect.NewResponse(&personaldataapiv1.ListKnownServersResponse{
		Servers:      servers,
		HomeServerId: cloneString(directory.HomeServerId),
	}), nil
}

func (s *personalDataService) CreateKnownServer(ctx context.Context, req *connect.Request[personaldataapiv1.CreateKnownServerRequest]) (*connect.Response[personaldataapiv1.CreateKnownServerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	server, err := apiPersonalServerToStored(req.Msg.GetServer(), true)
	if err != nil {
		return nil, err
	}
	created, err := s.api.core.PersonalData.CreateServer(ctx, caller.UserID, server)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.CreateKnownServerResponse{Server: personalServerToAPI(created)}), nil
}

func (s *personalDataService) UpdateKnownServer(ctx context.Context, req *connect.Request[personaldataapiv1.UpdateKnownServerRequest]) (*connect.Response[personaldataapiv1.UpdateKnownServerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	input := req.Msg.GetServer()
	paths, err := knownServerUpdatePaths(req.Msg.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, err
	}
	updated, err := s.api.core.PersonalData.UpdateServer(ctx, caller.UserID, input.GetId(), func(current *personaldatav1.KnownServer) error {
		for _, path := range paths {
			switch path {
			case "url":
				canonical, err := canonicalPersonalServerURL(input.GetUrl())
				if err != nil {
					return err
				}
				current.Url = canonical
			case "name":
				current.Name = input.GetName()
			case "icon_url":
				current.IconUrl = cloneString(input.IconUrl)
			}
		}
		return nil
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.UpdateKnownServerResponse{Server: personalServerToAPI(updated)}), nil
}

func (s *personalDataService) DeleteKnownServer(ctx context.Context, req *connect.Request[personaldataapiv1.DeleteKnownServerRequest]) (*connect.Response[personaldataapiv1.DeleteKnownServerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.PersonalData.DeleteServer(ctx, caller.UserID, req.Msg.GetId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.DeleteKnownServerResponse{}), nil
}

func (s *personalDataService) SetHomeServer(ctx context.Context, req *connect.Request[personaldataapiv1.SetHomeServerRequest]) (*connect.Response[personaldataapiv1.SetHomeServerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	home, err := s.api.core.PersonalData.SetHomeServer(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&personaldataapiv1.SetHomeServerResponse{Server: personalServerToAPI(home)}), nil
}

func preferenceUpdatePaths(paths []string) ([]string, error) {
	return checkedUpdatePaths(paths, map[string]struct{}{"locale": {}, "timezone": {}, "time_format": {}})
}

func knownServerUpdatePaths(paths []string) ([]string, error) {
	return checkedUpdatePaths(paths, map[string]struct{}{"url": {}, "name": {}, "icon_url": {}})
}

func checkedUpdatePaths(paths []string, allowed map[string]struct{}) ([]string, error) {
	if len(paths) == 0 {
		return nil, invalidArgument("update_mask must contain at least one field")
	}
	for _, path := range paths {
		if _, ok := allowed[path]; !ok {
			return nil, invalidArgument("unsupported update_mask path: " + path)
		}
	}
	return paths, nil
}

func apiPersonalServerToStored(server *personaldataapiv1.KnownServer, create bool) (*personaldatav1.KnownServer, error) {
	canonical, err := canonicalPersonalServerURL(server.GetUrl())
	if err != nil {
		return nil, err
	}
	addedAt := server.GetAddedAt()
	if create || addedAt == nil || !addedAt.IsValid() {
		addedAt = timestamppb.Now()
	}
	return &personaldatav1.KnownServer{
		Id:      server.GetId(),
		Url:     canonical,
		Name:    server.GetName(),
		IconUrl: cloneString(server.IconUrl),
		AddedAt: addedAt,
	}, nil
}

func canonicalPersonalServerURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", invalidArgument("server.url must be an HTTP or HTTPS origin without credentials")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		parsed.Host = "[" + hostname + "]"
	} else {
		parsed.Host = hostname
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func personalPreferencesToAPI(stored *personaldatav1.Preferences) *personaldataapiv1.Preferences {
	result := &personaldataapiv1.Preferences{
		Locale:   cloneString(stored.Locale),
		Timezone: cloneString(stored.Timezone),
	}
	if stored.TimeFormat != nil {
		value := storedPersonalTimeFormatToAPI(stored.GetTimeFormat())
		result.TimeFormat = &value
	}
	return result
}

func personalServerToAPI(stored *personaldatav1.KnownServer) *personaldataapiv1.KnownServer {
	result := &personaldataapiv1.KnownServer{
		Id:      stored.GetId(),
		Url:     stored.GetUrl(),
		Name:    stored.GetName(),
		IconUrl: cloneString(stored.IconUrl),
	}
	if stored.GetAddedAt() != nil {
		result.AddedAt = proto.Clone(stored.GetAddedAt()).(*timestamppb.Timestamp)
	}
	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func apiPersonalTimeFormatToStored(value personaldataapiv1.TimeFormat) personaldatav1.TimeFormat {
	switch value {
	case personaldataapiv1.TimeFormat_TIME_FORMAT_12_HOUR:
		return personaldatav1.TimeFormat_TIME_FORMAT_12_HOUR
	case personaldataapiv1.TimeFormat_TIME_FORMAT_24_HOUR:
		return personaldatav1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return personaldatav1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}

func storedPersonalTimeFormatToAPI(value personaldatav1.TimeFormat) personaldataapiv1.TimeFormat {
	switch value {
	case personaldatav1.TimeFormat_TIME_FORMAT_12_HOUR:
		return personaldataapiv1.TimeFormat_TIME_FORMAT_12_HOUR
	case personaldatav1.TimeFormat_TIME_FORMAT_24_HOUR:
		return personaldataapiv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return personaldataapiv1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}
