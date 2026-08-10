package memory

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// systemSymlinkMap lists the platform-known system-level symlink
// normalizations. These are part of the OS layout (macOS /var -> /private/var
// etc.), not user-controlled external symlinks, so they are the only ancestor
// symlinks ever accepted; any other symlink ancestor fails closed.
var systemSymlinkMap = func() map[string]string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return map[string]string{
		"/var": "/private/var",
		"/tmp": "/private/tmp",
		"/etc": "/private/etc",
	}
}()

// secureRoot normalizes and validates the Store root:
//   - must be absolute;
//   - an existing root that is itself a symlink is resolved (macOS
//     /var -> /private/var), so the store always operates on the real
//     location;
//   - a missing root is created component by component from its nearest
//     existing ancestor, with every existing ancestor component vetted one
//     by one (verifyAncestorChain): only real directories and the known
//     system symlink mappings are accepted, so any other symlink — including
//     one that is not the root's direct parent — is rejected;
//   - an existing root directory must be 0700 (group/other bits fail
//     closed); a missing root is created 0700.
func secureRoot(root string) (string, error) {
	if root == "" {
		return "", storeError(CodePathUnsafe, "store root must not be empty")
	}
	if !filepath.IsAbs(root) {
		return "", storeError(CodePathUnsafe, "store root must be absolute")
	}
	clean := filepath.Clean(root)

	// The root itself already exists.
	fi, err := os.Lstat(clean)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			// The root itself is a symlink: resolve it (handles the
			// existing-store and /var-style cases). The resolved path is
			// fully dereferenced, so no ancestor vetting is needed.
			resolved, err := filepath.EvalSymlinks(clean)
			if err != nil {
				return "", storeError(CodePermissionDenied, "cannot resolve store root")
			}
			resolved = filepath.Clean(resolved)
			if err := checkDirPermissions(resolved); err != nil {
				return "", err
			}
			return resolved, nil
		}
		if !fi.IsDir() {
			return "", storeError(CodePathUnsafe, "store root is not a directory")
		}
		if err := checkDirPermissions(clean); err != nil {
			return "", err
		}
		if _, _, err := verifyAncestorChain(filepath.Dir(clean)); err != nil {
			return "", err
		}
		// Normalize system prefixes (e.g. /var -> /private/var). The
		// ancestor chain was vetted above, so only system mappings can
		// appear in the resolution.
		resolved, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return "", storeError(CodePermissionDenied, "cannot resolve store root")
		}
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", storeError(CodePermissionDenied, "cannot inspect store root")
	}

	// The root does not exist yet: vet the existing ancestor chain, then
	// create the missing components under the normalized ancestor, verifying
	// each one as a real directory (a symlink planted in the race window is
	// rejected). Passing the full clean path (not just its parent) makes the
	// root's own last component part of the missing list.
	ancestor, missing, err := verifyAncestorChain(clean)
	if err != nil {
		return "", err
	}
	cur := ancestor
	for _, comp := range missing {
		cur = filepath.Join(cur, comp)
		if err := os.Mkdir(cur, 0o700); err != nil {
			if !os.IsExist(err) {
				return "", storeError(CodePermissionDenied, "cannot create store root")
			}
		}
		fi, err := os.Lstat(cur)
		if err != nil {
			return "", storeError(CodePermissionDenied, "cannot inspect store root")
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", storeError(CodeSymlinkRejected, "store root component is a symlink")
		}
		if !fi.IsDir() {
			return "", storeError(CodePathUnsafe, "store root component is not a directory")
		}
	}
	return filepath.Clean(cur), nil
}

// verifyAncestorChain walks path component by component from the filesystem
// root down, Lstat-ing each component individually (so a symlink anywhere in
// the chain is seen even when deeper components exist). Only real
// directories and the known system symlink mappings are accepted; any other
// symlink is rejected. It returns the normalized nearest existing ancestor
// and the missing components below it (in order), or a store error.
func verifyAncestorChain(path string) (string, []string, error) {
	parts := splitPathComponents(path)
	cur := ""
	for i, comp := range parts {
		next := joinPathComponent(cur, comp)
		fi, err := os.Lstat(next)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				target, ok := systemSymlinkMap[next]
				if !ok {
					return "", nil, storeError(CodeSymlinkRejected, "store root ancestor is an external symlink")
				}
				// The mapping declares a fixed system layout (/var ->
				// /private/var): verify the symlink actually points where the
				// mapping says, so a replaced system symlink fails closed
				// instead of being silently followed to an arbitrary target.
				got, err := os.Readlink(next)
				if err != nil {
					return "", nil, storeError(CodeSymlinkRejected, "store root ancestor is an external symlink")
				}
				if !filepath.IsAbs(got) {
					got = filepath.Join(filepath.Dir(next), got)
				}
				if filepath.Clean(got) != target {
					return "", nil, storeError(CodeSymlinkRejected, "store root ancestor is an external symlink")
				}
				// The mapping target itself must be a real directory; if the
				// system mapping is ever replaced, fail closed.
				tfi, err := os.Lstat(target)
				if err != nil || tfi.Mode()&os.ModeSymlink != 0 || !tfi.IsDir() {
					return "", nil, storeError(CodeSymlinkRejected, "store root ancestor is an external symlink")
				}
				cur = target
				continue
			}
			if !fi.IsDir() {
				return "", nil, storeError(CodePathUnsafe, "store root ancestor is not a directory")
			}
			cur = next
			continue
		}
		if !os.IsNotExist(err) {
			return "", nil, storeError(CodePermissionDenied, "cannot inspect store root")
		}
		// First missing component: everything below it is missing too.
		return cur, parts[i:], nil
	}
	return cur, nil, nil
}

func splitPathComponents(p string) []string {
	var out []string
	for _, c := range strings.Split(filepath.Clean(p), string(filepath.Separator)) {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func joinPathComponent(cur, comp string) string {
	if cur == "" {
		return string(filepath.Separator) + comp
	}
	return filepath.Join(cur, comp)
}

// checkDirPermissions fails closed unless the directory is owned exclusively
// by the owner (0700): any group/other permission bit is rejected, and the
// directory is never silently chmod'ed.
func checkDirPermissions(dir string) error {
	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return storeError(CodePermissionDenied, "cannot inspect store directory")
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return storeError(CodeSymlinkRejected, "store directory is a symlink")
	}
	if !fi.IsDir() {
		return storeError(CodePathUnsafe, "store directory is not a directory")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return storeError(CodeInsecurePermissions, "store directory permissions too permissive")
	}
	return nil
}

// validateFactKey checks a fact identity key: one to three ID components
// joined by "/", each restricted to the schema ID charset. Absolute paths,
// traversal, backslashes, control characters and empty components are
// rejected.
func validateFactKey(key string) ([]string, error) {
	if key == "" || len(key) > MaxFactKeyBytes {
		return nil, storeError(CodePathUnsafe, "invalid fact key")
	}
	comps := strings.Split(key, "/")
	if len(comps) < 1 || len(comps) > 3 {
		return nil, storeError(CodePathUnsafe, "invalid fact key")
	}
	for _, c := range comps {
		if err := validateID(c, "fact id"); err != nil {
			return nil, storeError(CodePathUnsafe, "invalid fact id")
		}
	}
	return comps, nil
}

// secureJoin walks root + components, rejecting symlinks and type changes at
// every existing component and creating missing directories (0700) in create
// mode. When lastIsFile, the final component is the fact file name and is
// never created here (atomic create follows); otherwise every component is a
// directory and missing ones are created.
func secureJoin(root string, comps []string, creating, lastIsFile bool) (string, error) {
	cur := root
	for i, comp := range comps {
		next := filepath.Join(cur, comp)
		fi, err := os.Lstat(next)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", storeError(CodeSymlinkRejected, "symlink component rejected")
			}
			last := i == len(comps)-1
			if last && lastIsFile {
				if !fi.Mode().IsRegular() {
					return "", storeError(CodePathUnsafe, "target is not a regular file")
				}
				if fi.Mode().Perm()&0o077 != 0 {
					return "", storeError(CodePermissionDenied, "target file permissions too permissive")
				}
			} else {
				if !fi.IsDir() {
					return "", storeError(CodePathUnsafe, "path component is not a directory")
				}
				if fi.Mode().Perm()&0o077 != 0 {
					return "", storeError(CodeInsecurePermissions, "directory permissions too permissive")
				}
			}
			cur = next
			continue
		}
		if !os.IsNotExist(err) {
			return "", storeError(CodePermissionDenied, "cannot inspect store path")
		}
		if !creating {
			return "", storeError(CodeNotFound, "fact not found")
		}
		if i == len(comps)-1 && lastIsFile {
			// Missing target file: atomic create will follow.
			cur = next
			continue
		}
		if err := os.Mkdir(next, 0o700); err != nil {
			if !os.IsExist(err) {
				return "", storeError(CodePermissionDenied, "cannot create store directory")
			}
			// Created concurrently: verify it is a real directory, not a
			// symlink planted in the race window.
			fi, err := os.Lstat(next)
			if err != nil {
				return "", storeError(CodePermissionDenied, "cannot inspect store path")
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", storeError(CodeSymlinkRejected, "symlink component rejected")
			}
			if !fi.IsDir() {
				return "", storeError(CodePathUnsafe, "path component is not a directory")
			}
		}
		cur = next
	}
	return cur, nil
}
