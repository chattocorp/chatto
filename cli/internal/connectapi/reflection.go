package connectapi

import (
	"fmt"
	"net/http"
	"sort"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
)

var publicReflectionServiceNames = []string{
	authv1connect.ExternalIdentityAuthServiceName,
	discoveryv1connect.ServerDiscoveryServiceName,
	apiv1connect.MyAccountServiceName,
	apiv1connect.AssetServiceName,
	apiv1connect.AssetUploadServiceName,
	adminv1connect.AdminDiagnosticsServiceName,
	adminv1connect.AdminEventLogServiceName,
	adminv1connect.AdminUserServiceName,
	adminv1connect.AdminPermissionServiceName,
	adminv1connect.AdminRoleServiceName,
	adminv1connect.AdminRoomLayoutServiceName,
	adminv1connect.AdminServerServiceName,
	apiv1connect.MessageServiceName,
	apiv1connect.MessageSearchServiceName,
	apiv1connect.NotificationPreferencesServiceName,
	apiv1connect.NotificationServiceName,
	apiv1connect.PushNotificationServiceName,
	apiv1connect.RoleServiceName,
	apiv1connect.RoomDirectoryServiceName,
	apiv1connect.RoomServiceName,
	apiv1connect.ServerServiceName,
	apiv1connect.ThreadServiceName,
	apiv1connect.UserServiceName,
	apiv1connect.ViewerServiceName,
	apiv1connect.VoiceCallServiceName,
}

func reflectionHandlers(options []connect.HandlerOption) []Handler {
	resolver := mustPublicReflectionResolver(publicReflectionServiceNames)
	reflector := grpcreflect.NewReflector(
		grpcreflect.NamerFunc(func() []string {
			return append([]string(nil), publicReflectionServiceNames...)
		}),
		grpcreflect.WithDescriptorResolver(resolver),
		grpcreflect.WithExtensionResolver(publicExtensionResolver{files: resolver}),
	)
	v1Path, v1Handler := grpcreflect.NewHandlerV1(reflector, options...)
	v1AlphaPath, v1AlphaHandler := grpcreflect.NewHandlerV1Alpha(reflector, options...)
	return []Handler{
		newPublicReflectionHandler(v1Path, v1Handler),
		newPublicReflectionHandler(v1AlphaPath, v1AlphaHandler),
	}
}

func newPublicReflectionHandler(path string, handler http.Handler) Handler {
	return Handler{
		ServicePath: path,
		Handler:     handler,
		AuthPolicy:  AuthPolicyPublic,
	}
}

func mustPublicReflectionResolver(serviceNames []string) *protoregistry.Files {
	resolver, err := publicReflectionResolver(serviceNames)
	if err != nil {
		panic(err)
	}
	return resolver
}

func publicReflectionResolver(serviceNames []string) (*protoregistry.Files, error) {
	seen := make(map[string]*descriptorpb.FileDescriptorProto)
	var addFile func(protoreflect.FileDescriptor)
	addFile = func(file protoreflect.FileDescriptor) {
		if file == nil || file.IsPlaceholder() {
			return
		}
		path := file.Path()
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = protodesc.ToFileDescriptorProto(file)
		imports := file.Imports()
		for i := 0; i < imports.Len(); i++ {
			addFile(imports.Get(i).FileDescriptor)
		}
	}

	for _, serviceName := range serviceNames {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(serviceName))
		if err != nil {
			return nil, fmt.Errorf("find service descriptor %q: %w", serviceName, err)
		}
		service, ok := desc.(protoreflect.ServiceDescriptor)
		if !ok {
			return nil, fmt.Errorf("descriptor %q is %T, want protoreflect.ServiceDescriptor", serviceName, desc)
		}
		addFile(service.ParentFile())
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	files := make([]*descriptorpb.FileDescriptorProto, 0, len(paths))
	for _, path := range paths {
		files = append(files, seen[path])
	}
	resolver, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: files})
	if err != nil {
		return nil, fmt.Errorf("build public reflection descriptor resolver: %w", err)
	}
	return resolver, nil
}

type publicExtensionResolver struct {
	files protodesc.Resolver
}

func (r publicExtensionResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	extension, err := protoregistry.GlobalTypes.FindExtensionByName(field)
	if err != nil {
		return nil, err
	}
	if !r.allows(extension.TypeDescriptor()) {
		return nil, protoregistry.NotFound
	}
	return extension, nil
}

func (r publicExtensionResolver) FindExtensionByNumber(message protoreflect.FullName, number protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	extension, err := protoregistry.GlobalTypes.FindExtensionByNumber(message, number)
	if err != nil {
		return nil, err
	}
	if !r.allows(extension.TypeDescriptor()) {
		return nil, protoregistry.NotFound
	}
	return extension, nil
}

func (r publicExtensionResolver) RangeExtensionsByMessage(message protoreflect.FullName, f func(protoreflect.ExtensionType) bool) {
	protoregistry.GlobalTypes.RangeExtensionsByMessage(message, func(extension protoreflect.ExtensionType) bool {
		if !r.allows(extension.TypeDescriptor()) {
			return true
		}
		return f(extension)
	})
}

func (r publicExtensionResolver) allows(desc protoreflect.Descriptor) bool {
	if desc == nil || desc.ParentFile() == nil {
		return false
	}
	_, err := r.files.FindFileByPath(desc.ParentFile().Path())
	return err == nil
}
