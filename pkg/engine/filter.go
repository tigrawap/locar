package engine

func (e *Explorer) isNotIncluded(path string) bool {
	if len(e.includes) != 0 {
		for _, include := range e.includes {
			if include.Match(path) {
				return false
			}
		}
		return true
	}
	return false
}

func (e *Explorer) isExcluded(path string) bool {
	for _, exclude := range e.excludes {
		if exclude.Match(path) {
			return true
		}
	}
	return false
}
