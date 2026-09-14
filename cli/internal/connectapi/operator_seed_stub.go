//go:build !bootstrap && !test_endpoints

package connectapi

import "connectrpc.com/connect"

// Production builds do not mount synthetic data generation.
func (a *API) operatorSeedHandlers(_ []connect.HandlerOption) []Handler { return nil }
