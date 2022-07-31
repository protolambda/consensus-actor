package yolo

type ValidatorRemapping struct {
	// human readable name of the remapping
	Name string `json:"name"`

	// single byte to use as prefix in the DB to distinguish from other remappings
	DbKey uint8 `json:"db_key"`

	// validator index -> Y axis index
	// Any Y pixel that is >= len(Remapping) should be appended at the end.
	// The default remapping is simply empty: original validator index order is preserved.
	Remapping []uint64 `json:"remapping"`
}

// TODO: if we store performance post-unshuffle (by validator index) then we can't make a layer-type that shows grouping committees without more expensive shuffling work.
