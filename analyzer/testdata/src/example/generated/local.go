package generated

type Local struct { // want Local:"&\\{false false\\}"
	private int
}

func build() {
	_ = &Message{Name: "local"}
	_ = new(Message)
	_ = Local{} // want "encapsulated struct example/generated.Local may only be constructed"
}
