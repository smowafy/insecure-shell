package main

type HandlerSet map[ConnectionHandler]struct{}

func NewHandlerSet() HandlerSet {
	return make(map[ConnectionHandler]struct{}, 0)
}

func (s HandlerSet) Add(element ConnectionHandler) {
	s[element] = struct{}{}
}

func (s HandlerSet) Remove(element ConnectionHandler) {
	delete(s, element)
}
