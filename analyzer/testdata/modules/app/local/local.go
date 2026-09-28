package local

type Hidden struct { // want Hidden:"&\\{false false example.com/app\\}"
	private int
}

func (h *Hidden) Private() int { return h.private }
