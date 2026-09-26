package factoryall

type First struct { hidden int } // want First:"&\\{true\\}"

func (f First) Clone() *First { return &First{} }

type Second struct { hidden int } // want Second:"&\\{true\\}"

type Maker struct{}

func (Maker) MakeSecond() *Second { return new(Second) }

func (Maker) Orphan() *First {
	_ = &First{} // want "encapsulated struct example/factoryall.First may only be constructed"
	return nil
}
