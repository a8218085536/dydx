package dydxtrading

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
)

const (
	protoWireVarint  = 0
	protoWireBytes   = 2
	protoWireFixed32 = 5
)

var errProtoFieldNotFound = errors.New("protobuf field not found")

type protoField struct {
	Number uint64
	Wire   uint64
	Value  []byte
}

type protoReader struct {
	data []byte
	pos  int
}

func newProtoReader(data []byte) *protoReader {
	return &protoReader{data: data}
}

func (r *protoReader) next() (protoField, bool, error) {
	if r.pos >= len(r.data) {
		return protoField{}, false, nil
	}
	tag, err := r.readUvarint()
	if err != nil {
		return protoField{}, false, err
	}
	field := protoField{Number: tag >> 3, Wire: tag & 0x7}
	switch field.Wire {
	case protoWireVarint:
		start := r.pos
		if _, err := r.readUvarint(); err != nil {
			return protoField{}, false, err
		}
		field.Value = r.data[start:r.pos]
	case protoWireBytes:
		length, err := r.readUvarint()
		if err != nil {
			return protoField{}, false, err
		}
		if length > uint64(len(r.data)-r.pos) {
			return protoField{}, false, io.ErrUnexpectedEOF
		}
		field.Value = r.data[r.pos : r.pos+int(length)]
		r.pos += int(length)
	case protoWireFixed32:
		if len(r.data)-r.pos < 4 {
			return protoField{}, false, io.ErrUnexpectedEOF
		}
		field.Value = r.data[r.pos : r.pos+4]
		r.pos += 4
	default:
		return protoField{}, false, fmt.Errorf("unsupported protobuf wire type %d", field.Wire)
	}
	return field, true, nil
}

func (r *protoReader) readUvarint() (uint64, error) {
	value, n := binary.Uvarint(r.data[r.pos:])
	if n == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if n < 0 {
		return 0, errors.New("protobuf varint overflow")
	}
	r.pos += n
	return value, nil
}

func protoFields(data []byte) ([]protoField, error) {
	reader := newProtoReader(data)
	var fields []protoField
	for {
		field, ok, err := reader.next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return fields, nil
		}
		fields = append(fields, field)
	}
}

func protoEachField(data []byte, fn func(protoField) error) error {
	fields, err := protoFields(data)
	if err != nil {
		return err
	}
	for _, field := range fields {
		if err := fn(field); err != nil {
			return err
		}
	}
	return nil
}

func protoBytes(data []byte, fieldNumber uint64) ([]byte, bool, error) {
	var found []byte
	err := protoEachField(data, func(field protoField) error {
		if field.Number != fieldNumber {
			return nil
		}
		if field.Wire != protoWireBytes {
			return fmt.Errorf("protobuf field %d has wire %d, want bytes", fieldNumber, field.Wire)
		}
		if found == nil {
			found = field.Value
		}
		return nil
	})
	return found, found != nil, err
}

func protoString(data []byte, fieldNumber uint64) (string, bool, error) {
	value, ok, err := protoBytes(data, fieldNumber)
	if err != nil || !ok {
		return "", ok, err
	}
	return string(value), true, nil
}

func protoUvarint(data []byte, fieldNumber uint64) (uint64, bool, error) {
	var found bool
	var value uint64
	err := protoEachField(data, func(field protoField) error {
		if field.Number != fieldNumber {
			return nil
		}
		if field.Wire != protoWireVarint {
			return fmt.Errorf("protobuf field %d has wire %d, want varint", fieldNumber, field.Wire)
		}
		if !found {
			decoded, n := binary.Uvarint(field.Value)
			if n <= 0 {
				return errors.New("invalid protobuf varint")
			}
			value = decoded
			found = true
		}
		return nil
	})
	return value, found, err
}

func protoFixed32(data []byte, fieldNumber uint64) (uint32, bool, error) {
	var found bool
	var value uint32
	err := protoEachField(data, func(field protoField) error {
		if field.Number != fieldNumber {
			return nil
		}
		if field.Wire != protoWireFixed32 {
			return fmt.Errorf("protobuf field %d has wire %d, want fixed32", fieldNumber, field.Wire)
		}
		if len(field.Value) != 4 {
			return io.ErrUnexpectedEOF
		}
		if !found {
			value = binary.LittleEndian.Uint32(field.Value)
			found = true
		}
		return nil
	})
	return value, found, err
}

func protoZigZag64(data []byte, fieldNumber uint64) (int64, bool, error) {
	value, ok, err := protoUvarint(data, fieldNumber)
	if err != nil || !ok {
		return 0, ok, err
	}
	return int64(value>>1) ^ -int64(value&1), true, nil
}

func protoSerializableIntString(data []byte, fieldNumber uint64) (string, bool, error) {
	value, ok, err := protoBytes(data, fieldNumber)
	if err != nil || !ok {
		return "", ok, err
	}
	var amount big.Int
	if err := amount.GobDecode(value); err != nil {
		return "", false, err
	}
	return amount.String(), true, nil
}
