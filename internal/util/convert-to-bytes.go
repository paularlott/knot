package util

import (
	"fmt"
	"strconv"
	"strings"
)

func ConvertToBytes(input string) (int64, error) {
    input = strings.ToUpper(input)
    length := len(input)
    lastChar := input[length-1:]
    value, err := strconv.ParseInt(input[:length-1], 10, 64)
    if err != nil {
        return 0, err
    }

    switch lastChar {
    case "G":
        value *= 1 << 30
    case "M":
        value *= 1 << 20
    case "K":
        value *= 1 << 10
    case "B":
        // value is already in bytes
        break

    default:
        // if the last character is not a letter, assume the value is already in bytes
        value, err = strconv.ParseInt(input, 10, 64)
        if err != nil {
            return 0, fmt.Errorf("invalid size: %s", input)
        }
    }

    return value, nil
}
