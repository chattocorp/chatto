package graph

func NewConfig(resolver ResolverRoot) Config {
	return Config{
		Resolvers: resolver,
		Directives: DirectiveRoot{
			Length: lengthDirective,
			Public: publicDirective,
		},
	}
}
