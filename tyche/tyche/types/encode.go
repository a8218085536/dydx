package types

import (
	"fmt"
)

func Encode(argsType []string, argv []interface{}) ([]byte, error) {
	if len(argsType) != len(argv) {
		return nil, fmt.Errorf("argument count mismatch: got %d for %d", len(argsType), len(argv))
	}

	args, err := parseArguments(argsType)
	if err != nil {
		return nil, err
	}

	var values []interface{}
	for i, arg := range args {
		v, err := convertType(argv[i], arg.Type)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}

	return args.Pack(values...)
}
