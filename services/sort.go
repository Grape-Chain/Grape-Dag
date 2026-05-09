package services

import "strings"

type Sort string

func (s Sort) isAsc() bool {
	return strings.ToUpper(string(s)) == "ASC"
}

func (s Sort) isDesc() bool {
	return strings.ToUpper(string(s)) == "DESC"
}
