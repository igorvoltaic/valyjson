package test_bool

// TestBool01 tests bool
//
//json:strict
type TestBool01 struct {
	Bool     bool  `json:"bl"`
	BlMaybe  bool  `json:"mb,omitempty"`
	RefBool  *bool `json:"refBool"`
	RefMaybe *bool `json:"refMaybe,omitempty"`

	DefBool bool `json:"defBool" default:"true"`
}

// TestBool02 tests bool
//
//json:optional
type TestBool02 struct {
	I TestInhBool `json:"i"`
	X TestInhBool `json:"x,omitempty"`
}

// TestInhBool tests inherited bool
//
//json:json
type TestInhBool bool
