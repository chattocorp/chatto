package http_server

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================================
// Query Depth Limit Tests
// ============================================================================

func TestGraphQL_QueryDepthLimit_AcceptsShallowQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { server { version } }`, nil)
	if len(resp.Errors) > 0 {
		t.Errorf("Expected shallow query to succeed, got errors: %v", resp.Errors)
	}
}

func TestGraphQL_QueryDepthLimit_AcceptsModerateQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { server { version config { serverName motd } } }`, nil)
	if len(resp.Errors) > 0 {
		t.Errorf("Expected moderate query to succeed, got errors: %v", resp.Errors)
	}
}

func TestGraphQL_QueryDepthLimit_RejectsDeeplyNestedQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Build a query that exceeds the depth limit of 12.
	// Use circular references: me → rooms → members → rooms → ... where each
	// nesting introduces a User → rooms → User cycle.
	query := `query {
		me {
			rooms {
				members {
					rooms {
						members {
							rooms {
								members {
									rooms {
										members {
											rooms {
												members {
													rooms {
														id
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	resp := env.doGraphQL(t, query, nil)

	if len(resp.Errors) == 0 {
		t.Fatal("Expected depth limit error for deeply nested query")
	}

	foundDepthError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "depth") && strings.Contains(e.Message, "exceeds the limit") {
			foundDepthError = true
		}
	}
	if !foundDepthError {
		t.Errorf("Expected depth limit error, got: %v", resp.Errors)
	}
}

func TestGraphQL_QueryDepthLimit_AllowsIntrospectionQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// The standard introspection query used by GraphQL playgrounds has deep
	// ofType nesting (7+ levels). It must not be rejected by the depth limit.
	query := `query IntrospectionQuery {
		__schema {
			queryType { name }
			mutationType { name }
			types {
				...FullType
			}
		}
	}
	fragment FullType on __Type {
		name
		fields(includeDeprecated: true) {
			name
			type { ...TypeRef }
		}
	}
	fragment TypeRef on __Type {
		name
		ofType { name ofType { name ofType { name
			ofType { name ofType { name ofType { name
				ofType { name }
			}}}
		}}}
	}`

	resp := env.doGraphQL(t, query, nil)

	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "depth") && strings.Contains(e.Message, "exceeds the limit") {
			t.Errorf("Introspection query should not be rejected by depth limit, got: %v", e.Message)
		}
	}
}

func TestGraphQL_QueryDepthLimit_InlineFragmentsDoNotAddDepth(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Inline fragments for type narrowing shouldn't count as additional depth.
	query := `query {
		server {
			version
			... on Server {
				config {
					serverName
				}
			}
		}
	}`

	resp := env.doGraphQL(t, query, nil)
	if len(resp.Errors) > 0 {
		t.Errorf("Expected query with inline fragments to succeed, got errors: %v", resp.Errors)
	}
}

func TestGraphQL_QueryDepthLimit_FragmentSpreadsCountDepth(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Same depth as the deeply nested test, but via a fragment spread.
	query := `
		query {
			me {
				...DeepUser
			}
		}

		fragment DeepUser on User {
			rooms {
				members {
					rooms {
						members {
							rooms {
								members {
									rooms {
										members {
											rooms {
												members {
													rooms {
														id
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	`

	resp := env.doGraphQL(t, query, nil)

	if len(resp.Errors) == 0 {
		t.Fatal("Expected depth limit error for deeply nested fragment")
	}

	foundDepthError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "depth") && strings.Contains(e.Message, "exceeds the limit") {
			foundDepthError = true
		}
	}
	if !foundDepthError {
		t.Errorf("Expected depth limit error, got: %v", resp.Errors)
	}
}

// ============================================================================
// Query Complexity Limit Tests
// ============================================================================

func TestGraphQL_ComplexityLimit_AcceptsSimpleQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { me { id login displayName } }`, nil)
	if len(resp.Errors) > 0 {
		t.Errorf("Expected simple query to succeed, got errors: %v", resp.Errors)
	}
}

func TestGraphQL_ComplexityLimit_RejectsExcessiveQuery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Build a query that requests many aliased copies of the same fields.
	// With FixedComplexityLimit(500), each field = 1 point.
	// 100 aliases × 6 fields = 600+ points
	var b strings.Builder
	b.WriteString("query {")
	for i := range 100 {
		b.WriteString(fmt.Sprintf("\n  i%d: server { version config { serverName motd welcomeMessage description } }", i))
	}
	b.WriteString("\n}")

	resp := env.doGraphQL(t, b.String(), nil)

	if len(resp.Errors) == 0 {
		t.Fatal("Expected complexity limit error for excessive query")
	}

	foundComplexityError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "complexity") && strings.Contains(e.Message, "exceeds the limit") {
			foundComplexityError = true
		}
	}
	if !foundComplexityError {
		t.Errorf("Expected complexity limit error, got: %v", resp.Errors)
	}
}
