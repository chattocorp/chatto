package graph

func NewConfig(resolver ResolverRoot) Config {
	return Config{
		Resolvers: resolver,
		Directives: DirectiveRoot{
			Authenticated: authenticatedDirective,
			Length:        lengthDirective,
		},
	}
}
