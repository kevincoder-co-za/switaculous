// Cuts a release: go run ./tools/release 0.0.2
//
// Writes the version to the VERSION file, commits it (signed off) and tags the
// commit v<version>, so the tag and the version every build reports always
// agree. With -push it also pushes the commit and the tag.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var releaseVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

func main() {
	push := flag.Bool("push", false, "push the release commit and tag to origin")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: go run ./tools/release [-push] <version>   e.g. go run ./tools/release 0.0.2")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	next := strings.TrimPrefix(flag.Arg(0), "v")
	// Only plain MAJOR.MINOR.PATCH: Windows installers and RPM packages
	// can't carry suffixes such as -beta.1 in their version field.
	if !releaseVersion.MatchString(next) {
		fail("%q is not a version like 0.0.2", flag.Arg(0))
	}
	current := readCurrentVersion()
	if !isNewer(next, current) {
		fail("%s is not newer than the current version %s", next, current)
	}
	tag := "v" + next

	if status := git("status", "--porcelain"); status != "" {
		fail("commit or stash your changes first; the release commit should only change VERSION:\n%s", status)
	}
	if _, err := runGit("rev-parse", "--quiet", "--verify", "refs/tags/"+tag); err == nil {
		fail("the tag %s already exists", tag)
	}

	if err := os.WriteFile("VERSION", []byte(next+"\n"), 0o644); err != nil {
		fail("couldn't write VERSION: %v", err)
	}
	git("add", "VERSION")
	git("commit", "-s", "-m", "Release "+tag)
	git("tag", "-a", tag, "-m", "Switaculous "+next)
	fmt.Printf("Released %s (was %s).\n", tag, current)

	if *push {
		git("push", "--atomic", "origin", "HEAD", "refs/tags/"+tag)
		fmt.Println("Pushed the release commit and tag.")
		return
	}
	fmt.Printf("Push it with:  git push --atomic origin HEAD %s\n", tag)
}

func readCurrentVersion() string {
	content, err := os.ReadFile("VERSION")
	if err != nil {
		fail("run this from the repository root (no VERSION file here): %v", err)
	}
	current := strings.TrimSpace(string(content))
	if !releaseVersion.MatchString(current) {
		fail("VERSION holds %q, which isn't a version like 0.0.1", current)
	}
	return current
}

func isNewer(next, current string) bool {
	a := releaseVersion.FindStringSubmatch(next)
	b := releaseVersion.FindStringSubmatch(current)
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return false
}

func runGit(args ...string) (string, error) {
	output, err := exec.Command("git", args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func git(args ...string) string {
	output, err := runGit(args...)
	if err != nil {
		fail("git %s failed:\n%s", strings.Join(args, " "), output)
	}
	return output
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "release: "+format+"\n", args...)
	os.Exit(1)
}
