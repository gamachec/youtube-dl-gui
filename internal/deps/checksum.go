package deps

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseChecksums parses a checksums file made of "<hash>  <name>" lines
// (a leading "*" on the name marks binary mode and is dropped). A line with
// only a hash is stored under the empty name. The PowerShell Get-FileHash
// layout ("Hash : <hex>", used by deno) is also understood as a bare hash.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		switch len(fields) {
		case 0:
		case 1:
			sums[""] = strings.ToLower(fields[0])
		case 3:
			if strings.EqualFold(fields[0], "hash") && fields[1] == ":" {
				sums[""] = strings.ToLower(fields[2])
				continue
			}
			fallthrough
		default:
			name := strings.TrimPrefix(fields[len(fields)-1], "*")
			sums[name] = strings.ToLower(fields[0])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return sums, nil
}

// lookupChecksum returns the hash for name, falling back to a bare hash.
func lookupChecksum(sums map[string]string, name string) (string, bool) {
	if h, ok := sums[name]; ok {
		return h, true
	}
	h, ok := sums[""]
	return h, ok
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
