package config

import "github.com/c2h5oh/datasize"

// ByteSize is an unsigned byte count. It uses datasize formats and binary
// multipliers, with additional support for IEC unit names such as MiB.
type ByteSize datasize.ByteSize

// UnmarshalText decodes TOML and environment values with IEC suffix support.
func (s *ByteSize) UnmarshalText(text []byte) error {
	return (*datasize.ByteSize)(s).UnmarshalText([]byte(normalizeIECByteUnit(string(text))))
}

// MarshalText keeps the existing datasize representation in generated TOML.
func (s ByteSize) MarshalText() ([]byte, error) {
	return datasize.ByteSize(s).MarshalText()
}
