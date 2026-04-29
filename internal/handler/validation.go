package handler

// Pagination defaults and limits.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ClampPagination ensures page >= 1 and pageSize is within [1, MaxPageSize].
// It returns the clamped values, using DefaultPageSize when pageSize is zero.
func ClampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}
