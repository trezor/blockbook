package build

// Port of build/scripts/rpcauth.py to Go.
//
// This reimplements the salt/password/HMAC generation of Bitcoin Core's
// rpcauth.py using only the Go standard library, so blockbookgen does not need
// a Python interpreter on PATH (which is awkward on native Windows builds).
//
// The original Python:
//
//	def generate_salt():
//	    cryptogen = SystemRandom()
//	    salt_sequence = [cryptogen.randrange(256) for _ in range(16)]
//	    return ''.join([format(r, 'x') for r in salt_sequence])
//
//	def generate_password():
//	    return base64.urlsafe_b64encode(os.urandom(32)).decode('utf-8')
//
//	def password_to_hmac(salt, password):
//	    m = hmac.new(bytearray(salt, 'utf-8'), bytearray(password, 'utf-8'), 'SHA256')
//	    return m.hexdigest()
//
//	# rpcauth={user}:{salt}${hmac}
//
// Standard library mapping:
//   - crypto/rand        -> os.urandom / SystemRandom (cryptographically secure)
//   - encoding/base64    -> base64.urlsafe_b64encode (URLEncoding, std padding)
//   - crypto/hmac+sha256 -> hmac.new(..., 'SHA256')
//   - strconv.FormatInt  -> format(r, 'x') (NOT encoding/hex, see generateSalt)
import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
)

// generateSalt mirrors rpcauth.py's generate_salt().
//
// Note the deliberate quirk: Python's format(r, 'x') formats each byte as hex
// WITHOUT zero padding, so a byte value of 5 becomes "5", not "05". This makes
// the salt a variable-length string, not a fixed 32-character hex string. We
// replicate that exactly with strconv.FormatInt; encoding/hex would zero-pad
// and produce a different (incompatible) salt.
func generateSalt() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	salt := ""
	for _, b := range raw {
		salt += strconv.FormatInt(int64(b), 16)
	}
	return salt, nil
}

// generatePassword mirrors rpcauth.py's generate_password(): 32 random bytes,
// URL-safe base64 encoded. Python's urlsafe_b64encode uses '-'/'_' and keeps
// '=' padding, matching Go's base64.URLEncoding.
func generatePassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.URLEncoding.EncodeToString(raw), nil
}

// passwordToHMAC mirrors rpcauth.py's password_to_hmac(): HMAC-SHA256 keyed by
// the salt string, over the password string, returned as a lowercase hex digest.
func passwordToHMAC(salt, password string) string {
	m := hmac.New(sha256.New, []byte(salt))
	m.Write([]byte(password))
	return hex.EncodeToString(m.Sum(nil))
}

// RPCAuth builds the "rpcauth=user:salt$hmac" line and returns the (possibly
// generated) plaintext password alongside it, mirroring rpcauth.py's main().
// If pass is empty a random password is generated.
func RPCAuth(user, pass string) (line, password string, err error) {
	salt, err := generateSalt()
	if err != nil {
		return "", "", err
	}

	password = pass
	if password == "" {
		password, err = generatePassword()
		if err != nil {
			return "", "", err
		}
	}

	passwordHMAC := passwordToHMAC(salt, password)
	line = fmt.Sprintf("rpcauth=%s:%s$%s", user, salt, passwordHMAC)
	return line, password, nil
}
