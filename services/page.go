package services

type Page struct {
	Number int
	Size   int
}

func (p Page) Offset() int {
	return p.Size * p.Number
}

func (p Page) Next() Page {
	p.Number++
	return p
}
