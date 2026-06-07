package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

func publicDirective(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
	return next(ctx)
}

func DefaultAuthFieldMiddleware(ctx context.Context, next graphql.Resolver) (any, error) {
	if publicField(graphql.GetFieldContext(ctx)) {
		return next(ctx)
	}

	if _, err := requireAuth(ctx); err != nil {
		return nil, err
	}
	return next(ctx)
}

func publicField(fc *graphql.FieldContext) bool {
	if fc == nil {
		return false
	}
	if strings.HasPrefix(fc.Object, "__") || strings.HasPrefix(fc.Field.Name, "__") {
		return true
	}
	return fc.Field.Definition != nil && fc.Field.Definition.Directives.ForName("public") != nil
}

func lengthDirective(ctx context.Context, obj any, next graphql.Resolver, min *int32, max int32, message *string) (any, error) {
	res, err := next(ctx)
	if err != nil {
		return nil, err
	}

	value, ok := stringDirectiveValue(res)
	if !ok {
		return res, nil
	}

	if min != nil && max < *min {
		return nil, fmt.Errorf("invalid length directive configuration: min %d exceeds max %d", *min, max)
	}
	if min != nil && len(value) < int(*min) {
		return nil, lengthDirectiveError(message, "must be at least %d bytes", *min)
	}
	if len(value) > int(max) {
		return nil, lengthDirectiveError(message, "must be at most %d bytes", max)
	}

	return res, nil
}

func stringDirectiveValue(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", false
	case string:
		return v, true
	case *string:
		if v == nil {
			return "", false
		}
		return *v, true
	default:
		return "", false
	}
}

func lengthDirectiveError(message *string, fallback string, args ...any) error {
	if message != nil && *message != "" {
		return fmt.Errorf("%s", *message)
	}
	return fmt.Errorf(fallback, args...)
}
