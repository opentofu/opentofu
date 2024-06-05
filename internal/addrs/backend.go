package addrs

type Backend struct {
	referenceable
	Type string
}

func (b Backend) String() string {
	return "backend." + b.Type
}

func (b Backend) UniqueKey() UniqueKey {
	return b // A Backend is its own UniqueKey
}

func (b Backend) uniqueKeySigil() {}
