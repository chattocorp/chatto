package connectapi

import apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"

func apiPagination(page *apiv1.PageRequest, defaultLimit, maxLimit int) (int, int) {
	limitVal := defaultLimit
	if page.GetLimit() > 0 {
		limitVal = int(page.GetLimit())
	}
	if limitVal > maxLimit {
		limitVal = maxLimit
	}
	offsetVal := 0
	if page.GetOffset() > 0 {
		offsetVal = int(page.GetOffset())
	}
	return limitVal, offsetVal
}

func apiPageInfo(totalCount int, hasMore bool) *apiv1.PageInfo {
	return &apiv1.PageInfo{
		TotalCount: int64(totalCount),
		HasMore:    hasMore,
	}
}

func apiSlicePage[T any](items []T, limit, offset int) ([]T, int, bool) {
	total := len(items)
	if offset >= total {
		return []T{}, total, false
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end], total, end < total
}
