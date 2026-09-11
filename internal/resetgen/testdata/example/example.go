package example

import (
	"bytes"
	"time"
)

// generate:reset
type ResetableStruct struct {
	i     int
	str   string
	strP  *string
	s     []int
	m     map[string]string
	child *ResetableStruct
}

// Wrapper проверяет вложенные и чужие типы.
//
// generate:reset
type Wrapper struct {
	rs    ResetableStruct
	buf   bytes.Buffer
	at    time.Time
	names *[]string
	kind  Kind
	b     bool
	f     float64
	pp    **ResetableStruct
	pm    *map[string]int
	pt    *time.Time
	emb   Embeds
	plain Plain
}

// Plain без маркера: Reset для неё не генерируется.
type Plain struct {
	n int
}

// Embeds получает Reset от встроенного *bytes.Buffer.
type Embeds struct {
	*bytes.Buffer
	n int
}

type Kind string

type (
	// generate:reset
	InGroup struct {
		n int
	}

	NotMarked struct {
		n int
	}
)
