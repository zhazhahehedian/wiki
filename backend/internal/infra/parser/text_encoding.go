package parser

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func decodeTextBytes(src []byte) (string, error) {
	src = bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(src) {
		return string(src), nil
	}

	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(src)
	if err != nil {
		return "", fmt.Errorf("decode text as UTF-8 or GB18030: %w", err)
	}
	return string(decoded), nil
}
