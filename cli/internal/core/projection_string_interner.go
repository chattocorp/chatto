package core

type projectionStringInterner map[string]string

func newProjectionStringInterner() projectionStringInterner {
	return make(projectionStringInterner)
}

func (i projectionStringInterner) intern(value string) string {
	if value == "" {
		return ""
	}
	if existing, ok := i[value]; ok {
		return existing
	}
	i[value] = value
	return value
}
