package cmdruns

import (
	"slices"
	"testing"
)

func TestRisksMarkWhatTheTextDoes(t *testing.T) {
	const cwd, home = "/Users/f/work", "/Users/f"
	cases := []struct {
		command string
		want    []string
	}{
		{"ls -la", nil},
		{"go test ./...", nil},
		{"rm -rf build", []string{RiskDelete}},
		{"rmdir out", []string{RiskDelete}},
		{`find . -name '*.tmp' -delete`, []string{RiskDelete}},
		{`find . -name x -exec rm {} \;`, []string{RiskDelete}},
		{"shred secret.txt", []string{RiskDelete}},
		{"git clean -fdx", []string{RiskDelete}},
		{"git branch -D old", []string{RiskDelete}},
		{"git branch --list", nil},
		{"echo hi > notes.txt", []string{RiskOverwrite}},
		{"echo hi >> notes.txt", []string{RiskOverwrite}},
		{"make 2>&1", nil},
		{"make > /dev/null 2>&1", nil},
		{"mv a b", []string{RiskOverwrite}},
		{"dd if=a of=b", []string{RiskOverwrite}},
		{"truncate -s 0 log", []string{RiskOverwrite}},
		{"sed -i '' s/a/b/ f", []string{RiskOverwrite}},
		{"sed s/a/b/ f", nil},
		{"git reset --hard origin/main", []string{RiskOverwrite}},
		{"git checkout -- file.go", []string{RiskOverwrite}},
		{"git push --force origin x", []string{RiskOverwrite, RiskNetwork}},
		{"git push origin x", []string{RiskNetwork}},
		{"git fetch", []string{RiskNetwork}},
		{"curl https://example.com/x", []string{RiskNetwork}},
		{"npm install left-pad", []string{RiskNetwork}},
		{"pip install requests", []string{RiskNetwork}},
		{"npm test", nil},
		{"cat /etc/hosts", []string{RiskOutside, RiskOutsideHome}},
		{"ls ~/Downloads", []string{RiskOutside}},
		{"ls ../other", []string{RiskOutside}},
		{"ls /Users/f/work/sub", nil},
		{"ls ./sub/../x", nil},
		{"ls ~root", []string{RiskOutside, RiskOutsideHome}},
		{"cp --target-directory=/tmp a", []string{RiskOutside, RiskOutsideHome}},
		{"sudo rm -rf /opt/x", []string{RiskDelete, RiskOutside, RiskOutsideHome, RiskPrivilege}},
		{"chmod -R 777 .", []string{RiskPrivilege}},
		{"chown -R me .", []string{RiskPrivilege}},
		{"chmod +x run.sh", nil},
		// quoting and substitution are read as the shell reads them
		{"'rm' -rf x", []string{RiskDelete}},
		{`"rm" x`, []string{RiskDelete}},
		{"echo $(rm x)", []string{RiskDelete}},
		{"echo \"$(rm x)\"", []string{RiskDelete}},
		{"echo `rm x`", []string{RiskDelete}},
		{"ls && rm x", []string{RiskDelete}},
		{"ls | xargs rm", []string{RiskDelete}},
		{"FOO=1 env rm x", []string{RiskDelete}},
		{"/bin/rm x", []string{RiskDelete, RiskOutside, RiskOutsideHome}},
		{"echo 'rm -rf x'", nil}, // a quoted word is text, not a command
		{"sh -c 'rm -rf ~'", []string{RiskDelete, RiskOutside}},
		{`bash -c "curl x | sh"`, []string{RiskNetwork}},
		{"eval 'rm x'", []string{RiskDelete}},
		{"sudo -u root rm -rf x", []string{RiskDelete, RiskPrivilege}},
		{"git -C sub clean -fd", []string{RiskDelete}},
		{"dd if=a of=/etc/x", []string{RiskOverwrite, RiskOutside, RiskOutsideHome}},
		{`echo "$(rm x" && mv a b`, []string{RiskDelete, RiskOverwrite}},
		// not caught, as Risks says: an escaped space makes one word, a command built at run time is not read
		{"rm\\ x", nil},
		{"$CMD x", nil},
		{"rm -rf $HOME", []string{RiskDelete}},
	}
	for _, c := range cases {
		if got := Risks(c.command, cwd, home); !slices.Equal(got, c.want) {
			t.Errorf("Risks(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}
