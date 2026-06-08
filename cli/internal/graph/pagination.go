package graph

func paginationArgs(limit *int32, offset *int32, defaultLimit int, maxLimit int) (int, int) {
	limitVal := defaultLimit
	if limit != nil && *limit > 0 {
		limitVal = int(*limit)
	}
	if limitVal > maxLimit {
		limitVal = maxLimit
	}

	offsetVal := 0
	if offset != nil && *offset >= 0 {
		offsetVal = int(*offset)
	}

	return limitVal, offsetVal
}

func firstNArg(first *int32, defaultLimit int, maxLimit int) int {
	limit, _ := paginationArgs(first, nil, defaultLimit, maxLimit)
	return limit
}

func paginateSlice[T any](items []T, limit int, offset int) ([]T, int, bool) {
	totalCount := len(items)
	if offset >= totalCount {
		return []T{}, totalCount, false
	}

	page := items[offset:]
	if limit > 0 && len(page) > limit {
		page = page[:limit]
	}

	return page, totalCount, offset+len(page) < totalCount
}
