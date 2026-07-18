package connectapi

import (
	"context"
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/idna"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	clientsyncapiv1 "hmans.de/chatto/internal/pb/chatto/clientsync/api/v1"
	clientsyncv1 "hmans.de/chatto/internal/pb/chatto/clientsync/v1"
)

type clientSyncService struct {
	api *API
}

func (s *clientSyncService) requireCaller(ctx context.Context) (Caller, error) {
	if !s.api.config.ClientSync.Enabled {
		return Caller{}, connect.NewError(
			connect.CodeUnimplemented,
			errors.New("client sync is disabled by the server operator"),
		)
	}
	return requireCaller(ctx)
}

func (s *clientSyncService) GetPreferences(ctx context.Context, _ *connect.Request[clientsyncapiv1.GetPreferencesRequest]) (*connect.Response[clientsyncapiv1.GetPreferencesResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	preferences, err := s.api.core.ClientSync.GetPreferences(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&clientsyncapiv1.GetPreferencesResponse{Preferences: clientSyncPreferencesToAPI(preferences)}), nil
}

func (s *clientSyncService) UpdatePreferences(ctx context.Context, req *connect.Request[clientsyncapiv1.UpdatePreferencesRequest]) (*connect.Response[clientsyncapiv1.UpdatePreferencesResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	paths, err := preferenceUpdatePaths(req.Msg.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, err
	}
	input := req.Msg.GetPreferences()
	if slices.Contains(paths, "timezone") && input.Timezone != nil {
		if input.GetTimezone() == "" {
			input.Timezone = nil
		} else if input.GetTimezone() == "Local" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("timezone must be a browser-compatible IANA time zone"))
		} else if _, err := time.LoadLocation(input.GetTimezone()); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("timezone must be a valid IANA time zone"))
		}
	}
	preferences, err := s.api.core.ClientSync.UpdatePreferences(ctx, caller.UserID, func(current *clientsyncv1.Preferences) error {
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
					value := apiClientSyncTimeFormatToStored(input.GetTimeFormat())
					current.TimeFormat = &value
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&clientsyncapiv1.UpdatePreferencesResponse{Preferences: clientSyncPreferencesToAPI(preferences)}), nil
}

func (s *clientSyncService) ListKnownServers(ctx context.Context, _ *connect.Request[clientsyncapiv1.ListKnownServersRequest]) (*connect.Response[clientsyncapiv1.ListKnownServersResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	directory, err := s.api.core.ClientSync.ListServers(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	servers := make([]*clientsyncapiv1.KnownServer, 0, len(directory.GetServers()))
	for _, server := range directory.GetServers() {
		servers = append(servers, clientSyncServerToAPI(server))
	}
	return connect.NewResponse(&clientsyncapiv1.ListKnownServersResponse{
		Servers:      servers,
		HomeServerId: cloneString(directory.HomeServerId),
	}), nil
}

func (s *clientSyncService) CreateKnownServer(ctx context.Context, req *connect.Request[clientsyncapiv1.CreateKnownServerRequest]) (*connect.Response[clientsyncapiv1.CreateKnownServerResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	server, err := apiClientSyncServerToStored(req.Msg.GetServer(), true)
	if err != nil {
		return nil, err
	}
	created, err := s.api.core.ClientSync.CreateServer(ctx, caller.UserID, server)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&clientsyncapiv1.CreateKnownServerResponse{Server: clientSyncServerToAPI(created)}), nil
}

func (s *clientSyncService) UpdateKnownServer(ctx context.Context, req *connect.Request[clientsyncapiv1.UpdateKnownServerRequest]) (*connect.Response[clientsyncapiv1.UpdateKnownServerResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	input := req.Msg.GetServer()
	paths, err := knownServerUpdatePaths(req.Msg.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, err
	}
	updated, err := s.api.core.ClientSync.UpdateServer(ctx, caller.UserID, input.GetId(), func(current *clientsyncv1.KnownServer) error {
		for _, path := range paths {
			switch path {
			case "url":
				canonical, err := canonicalClientSyncServerURL(input.GetUrl())
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
	return connect.NewResponse(&clientsyncapiv1.UpdateKnownServerResponse{Server: clientSyncServerToAPI(updated)}), nil
}

func (s *clientSyncService) DeleteKnownServer(ctx context.Context, req *connect.Request[clientsyncapiv1.DeleteKnownServerRequest]) (*connect.Response[clientsyncapiv1.DeleteKnownServerResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ClientSync.DeleteServer(ctx, caller.UserID, req.Msg.GetId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&clientsyncapiv1.DeleteKnownServerResponse{}), nil
}

func (s *clientSyncService) SetHomeServer(ctx context.Context, req *connect.Request[clientsyncapiv1.SetHomeServerRequest]) (*connect.Response[clientsyncapiv1.SetHomeServerResponse], error) {
	caller, err := s.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	home, err := s.api.core.ClientSync.SetHomeServer(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&clientsyncapiv1.SetHomeServerResponse{Server: clientSyncServerToAPI(home)}), nil
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

func apiClientSyncServerToStored(server *clientsyncapiv1.KnownServer, create bool) (*clientsyncv1.KnownServer, error) {
	canonical, err := canonicalClientSyncServerURL(server.GetUrl())
	if err != nil {
		return nil, err
	}
	addedAt := server.GetAddedAt()
	if create || addedAt == nil || !addedAt.IsValid() {
		addedAt = timestamppb.Now()
	}
	return &clientsyncv1.KnownServer{
		Id:      server.GetId(),
		Url:     canonical,
		Name:    server.GetName(),
		IconUrl: cloneString(server.IconUrl),
		AddedAt: addedAt,
	}, nil
}

func canonicalClientSyncServerURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", invalidArgument("server.url must be an HTTP or HTTPS origin without credentials")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", invalidArgument("server.url must be an HTTP or HTTPS origin without credentials")
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", invalidArgument("server.url must contain a valid hostname")
	}
	if net.ParseIP(hostname) == nil {
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil || hostname == "" {
			return "", invalidArgument("server.url must contain a valid hostname")
		}
	}
	hostname = strings.ToLower(hostname)
	port := parsed.Port()
	if port != "" {
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber > 65535 {
			return "", invalidArgument("server.url contains an invalid port")
		}
	}
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

func clientSyncPreferencesToAPI(stored *clientsyncv1.Preferences) *clientsyncapiv1.Preferences {
	result := &clientsyncapiv1.Preferences{
		Locale:   cloneString(stored.Locale),
		Timezone: cloneString(stored.Timezone),
	}
	if stored.TimeFormat != nil {
		value := storedClientSyncTimeFormatToAPI(stored.GetTimeFormat())
		result.TimeFormat = &value
	}
	return result
}

func clientSyncServerToAPI(stored *clientsyncv1.KnownServer) *clientsyncapiv1.KnownServer {
	result := &clientsyncapiv1.KnownServer{
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

func apiClientSyncTimeFormatToStored(value clientsyncapiv1.TimeFormat) clientsyncv1.TimeFormat {
	switch value {
	case clientsyncapiv1.TimeFormat_TIME_FORMAT_12_HOUR:
		return clientsyncv1.TimeFormat_TIME_FORMAT_12_HOUR
	case clientsyncapiv1.TimeFormat_TIME_FORMAT_24_HOUR:
		return clientsyncv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return clientsyncv1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}

func storedClientSyncTimeFormatToAPI(value clientsyncv1.TimeFormat) clientsyncapiv1.TimeFormat {
	switch value {
	case clientsyncv1.TimeFormat_TIME_FORMAT_12_HOUR:
		return clientsyncapiv1.TimeFormat_TIME_FORMAT_12_HOUR
	case clientsyncv1.TimeFormat_TIME_FORMAT_24_HOUR:
		return clientsyncapiv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return clientsyncapiv1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}
