package proxy

var defaultEncrypt func([]byte) ([]byte, error)

func SetEncryptor(fn func([]byte) ([]byte, error)) {
	defaultEncrypt = fn
}
