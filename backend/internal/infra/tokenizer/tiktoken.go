package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

type Tiktoken struct {
	enc *tiktoken.Tiktoken
}

var (
	defaultInstance *Tiktoken
	initOnce        sync.Once
	initErr         error
)

func Init(encoding string) error {
	initOnce.Do(func() {
		enc, err := tiktoken.GetEncoding(encoding)
		if err != nil {
			initErr = fmt.Errorf("get tiktoken encoding %q: %w", encoding, err)
			return
		}
		defaultInstance = &Tiktoken{enc: enc}
	})
	return initErr
}

func Default() *Tiktoken {
	if defaultInstance == nil {
		panic("tokenizer not initialized; call tokenizer.Init first")
	}
	return defaultInstance
}

func (t *Tiktoken) Count(s string) int {
	return len(t.enc.Encode(s, nil, nil))
}

func (t *Tiktoken) Encode(s string) []int {
	return t.enc.Encode(s, nil, nil)
}

func (t *Tiktoken) Decode(ids []int) string {
	return t.enc.Decode(ids)
}
