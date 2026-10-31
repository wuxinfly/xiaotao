package trae

import "errors"

var ErrEmptyCatalog = errors.New("trae model catalog is empty")

type HTTPError struct {
	Code int
}

func (e *HTTPError) Error() string {
	return "trae http " + itoa(e.Code)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
