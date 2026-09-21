package credential

import (
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/zalando/go-keyring"
)

type Store interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}
type System struct{}

func (System) Get(user string) (string, error) {
	s, e := keyring.Get("cn.seu.seusc", user)
	if errors.Is(e, keyring.ErrNotFound) {
		return "", nil
	}
	if e != nil {
		return "", apperr.New("CREDENTIAL_STORE_FAILED", "无法读取系统凭据存储")
	}
	return s, nil
}
func (System) Set(user, password string) error {
	if e := keyring.Set("cn.seu.seusc", user, password); e != nil {
		return apperr.New("CREDENTIAL_STORE_FAILED", "无法保存到系统凭据存储")
	}
	return nil
}
func (System) Delete(user string) error {
	if user == "" {
		return nil
	}
	e := keyring.Delete("cn.seu.seusc", user)
	if e != nil && !errors.Is(e, keyring.ErrNotFound) {
		return apperr.New("CREDENTIAL_STORE_FAILED", "无法删除系统凭据")
	}
	return nil
}
