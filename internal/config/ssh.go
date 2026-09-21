package config

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"strings"
)

const begin = "# BEGIN SEUSC MANAGED BLOCK"
const end = "# END SEUSC MANAGED BLOCK"

func ManagedBlock(existing, block string) (string, error) {
	n, m := strings.Count(existing, begin), strings.Count(existing, end)
	if n != m || n > 1 {
		return "", apperr.New("SSH_CONFIG_CONFLICT", "SSH 配置中的 SEUSC 标记不完整或重复")
	}
	if n == 1 {
		a, b := strings.Index(existing, begin), strings.Index(existing, end)
		if b < a {
			return "", errors.New("invalid managed SSH block")
		}
		lineStart := strings.LastIndex(existing[:a], "\n") + 1
		if strings.TrimSpace(existing[lineStart:a]) != "" {
			return "", errors.New("invalid managed SSH marker")
		}
		after := b + len(end)
		if after < len(existing) && existing[after] == '\r' {
			after++
		}
		if after < len(existing) && existing[after] == '\n' {
			after++
		}
		return existing[:lineStart] + block + existing[after:], nil
	}
	// Prepend: OpenSSH uses the first value found, so a preceding Host * must not override our settings.
	return block + existing, nil
}
func Key(p string) (ssh.Signer, error) {
	b, e := os.ReadFile(p)
	if e == nil {
		if err := platform.Protect(p); err != nil {
			return nil, err
		}
		return ssh.ParsePrivateKey(b)
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	_, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, e
	}
	block, e := ssh.MarshalPrivateKey(private, "SEUSC local key")
	if e != nil {
		return nil, e
	}
	if e = AtomicWrite(p, pem.EncodeToMemory(block), 0600); e != nil {
		return nil, e
	}
	if e = platform.Protect(p); e != nil {
		return nil, e
	}
	return ssh.NewSignerFromKey(private)
}
func EnsureSSH(p platform.Paths, c Config) (ssh.Signer, ssh.PublicKey, error) {
	host, e := Key(filepath.Join(p.DataDir, "ssh_host_ed25519_key"))
	if e != nil {
		return nil, nil, e
	}
	client, e := Key(filepath.Join(p.SSHDir, "seusc_ed25519"))
	if e != nil {
		return nil, nil, e
	}
	if e = AtomicWrite(filepath.Join(p.SSHDir, "seusc_ed25519.pub"), ssh.MarshalAuthorizedKey(client.PublicKey()), 0600); e != nil {
		return nil, nil, e
	}
	if e = WriteSSH(p, c, host.PublicKey()); e != nil {
		return nil, nil, e
	}
	return host, client.PublicKey(), nil
}
func WriteSSH(p platform.Paths, c Config, host ssh.PublicKey) error {
	file := filepath.Join(p.SSHDir, "config")
	b, e := os.ReadFile(file)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	// Conservatively reject a user-owned alias, including aliases mixed into a Host line.
	unmanaged := string(b)
	if strings.Count(unmanaged, begin) == 1 && strings.Count(unmanaged, end) == 1 {
		a, z := strings.Index(unmanaged, begin), strings.Index(unmanaged, end)
		if z >= a {
			unmanaged = unmanaged[:a] + unmanaged[z+len(end):]
		}
	}
	for _, line := range strings.Split(unmanaged, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && strings.EqualFold(f[0], "Host") {
			for _, name := range f[1:] {
				if name == c.SSH.Alias {
					return apperr.New("SSH_CONFIG_CONFLICT", "已有同名 SSH Host，请先重命名原有别名")
				}
			}
		}
	}
	keyPath := filepath.ToSlash(filepath.Join(p.SSHDir, "seusc_ed25519"))
	knownPath := filepath.ToSlash(filepath.Join(p.SSHDir, "seusc_known_hosts"))
	for _, v := range []string{keyPath, knownPath} {
		if strings.ContainsAny(v, "\"\r\n") {
			return errors.New("unsupported SSH file path")
		}
	}
	block := fmt.Sprintf("%s\nHost %s\n    HostName %s\n    Port %d\n    User seusc\n    IdentityFile \"%s\"\n    IdentitiesOnly yes\n    UserKnownHostsFile \"%s\"\n    StrictHostKeyChecking yes\n%s\n", begin, c.SSH.Alias, c.SSH.Listen, c.SSH.Port, keyPath, knownPath, end)
	updated, e := ManagedBlock(string(b), block)
	if e != nil {
		return e
	}
	known := fmt.Sprintf("[%s]:%d %s", c.SSH.Listen, c.SSH.Port, ssh.MarshalAuthorizedKey(host))
	if e = AtomicWrite(filepath.Join(p.SSHDir, "seusc_known_hosts"), []byte(known), 0600); e != nil {
		return e
	}
	if bytes.Equal(b, []byte(updated)) {
		return nil
	}
	if len(b) > 0 {
		backup := file + ".seusc-backup"
		if _, e = os.Stat(backup); os.IsNotExist(e) {
			if e = AtomicWrite(backup, b, 0600); e != nil {
				return e
			}
		}
	}
	return AtomicWrite(file, []byte(updated), 0600)
}
