package schema

func GetAs[T any](data ReadOnly, key string, defaultVal ...T) T {
	var zero T
	val, ok := data.Get(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return zero
	}
	if v, match := val.(T); match {
		return v
	}
	if len(defaultVal) > 0 {
		return defaultVal[0]
	}
	return zero
}
