package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	command := os.Args[1]
	switch command {
	case "init":
		Init()
	case "commit":
		Commit()
	case "test":
		get_lock("test.txt")
		fmt.Println("test")
		get_lock("test.txt")
	default:
		log.Fatal("Unknown command")
	}
}

type Tree struct {
	entries []Entry
	mode    string
}
type Entry struct {
	oid  string
	path string
}

func Commit() {
	// root, err := os.Getwd()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//git_path := path.Join(root, ".git")
	// objects_path := path.Join(git_path, "objects")
	// workspace in book
	workspace_path := "."
	ignore_arr := []string{".git", ".."}
	ignore := make(map[string]struct{})
	for _, s := range ignore_arr {
		ignore[s] = struct{}{}
	}
	var arr []string
	filepath.WalkDir(workspace_path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		_, ok := ignore[d.Name()]
		if d.IsDir() && ok {
			return filepath.SkipDir
		}
		if ok || d.Name() == workspace_path {
			return nil
		}
		arr = append(arr, path)
		return nil
	})
	// database in book
	filesystem := os.DirFS(workspace_path)
	entries := make([]Entry, len(arr))
	for i, s := range arr {
		file, _ := fs.ReadFile(filesystem, s)
		// make blob
		content := get_content_str("blob", file)
		oid := get_sha_str(content)
		// create dir and file
		write_object(oid, content)
		entries[i] = Entry{oid, s}
	}
	tree_oid := make_tree(entries)
	// Make Commit
	name := os.Getenv("GIT_AUTHOR_NAME")
	email := os.Getenv("GIT_AUTHOR_EMAIL")
	current_time := time.Now()
	timestamp := current_time.Unix()
	utc_offset := current_time.UTC().Format("-0700")
	author := name + " <" + email + ">" + " " + fmt.Sprintf("%d %s", timestamp, utc_offset)
	reader := bufio.NewReader(os.Stdin)
	message, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	commit_arr := []string{"tree " + tree_oid, "author " + author, "committer " + author, "", message}
	parent, err := read_head()
	if err != nil {
		log.Fatal(err)
	}
	if parent != "" {
		arr = append(commit_arr[:2], commit_arr[1:]...)
		commit_arr[1] = "parent " + parent
	}
	commit_str := strings.Join(commit_arr[:], "\n")
	commit_oid := get_sha_str(commit_str)
	commit_content := get_content_str("commit", []byte(commit_str))
	write_object(commit_oid, commit_content)
	update_head(commit_oid)
	// head_str := "[(root-commit) " + commit_oid + "]" + " " + message
	fmt.Println(commit_str)
}
func update_head(oid string) {
	file := get_lock(path.Join(".git", "HEAD"))
	file.WriteString(oid + "\n")
	file.Close()
	err := os.Rename(file.Name(), path.Join(".git", "HEAD"))
	if err != nil {
		log.Fatal(err)
	}
}
func get_lock(filepath string) *os.File {
	lock_path := filepath + ".lock"
	f, err := os.OpenFile(lock_path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		if os.IsExist(err) {
			log.Fatal("File already exists")
		} else if os.IsPermission(err) {
			log.Fatal("Permission denied")
		} else if os.IsNotExist(err) {
			log.Fatal("No such file or directory")
		} else {
			log.Fatal(err)
		}
	}
	return f
}
func read_head() (string, error) {
	filepath := path.Join(".git", "HEAD")
	f, err := os.Open(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	return scanner.Text(), nil

}
func make_tree(entries []Entry) string {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})
	tree_arr := make([]string, 0)
	for _, entry := range entries {
		fmt.Println(entry.path)
		tree_arr = append(tree_arr, get_entry_str("100644", entry))
	}
	tree_str := strings.Join(tree_arr, "")
	tree_content := get_content_str("tree", []byte(tree_str))
	tree_oid := get_sha_str(tree_content)
	write_object(tree_oid, tree_content)
	return tree_oid
}
func write_object(oid string, content string) {
	database_path := path.Join(".git", "objects")
	dir := path.Join(database_path, oid[:2])
	err := os.Mkdir(dir, fs.ModePerm)
	if err != nil {
		if os.IsExist(err) {
			return
		} else {
			log.Fatal(err)
		}
	}
	filepath := path.Join(dir, oid[2:])
	f, err := os.Create(filepath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	compressed := zlib_compress(content)
	_, err = f.Write(compressed)
	if err != nil {
		log.Fatal(err)
	}
}
func get_entry_str(mode string, entry Entry) string {
	compressed_oid, err := hex.DecodeString(entry.oid)
	if err != nil {
		log.Fatal(err)
	}
	entry_str := []byte(mode + " " + entry.path + "\x00")
	entry_str = append(entry_str, compressed_oid...)
	return string(entry_str)

}
func get_content_str(obj_type string, file []byte) string {
	return (obj_type + " " + fmt.Sprintf("%d", len(file)) + "\x00" + string(file))
}

// https://www.joeshaw.org/dont-defer-close-on-writable-files/
// Call w.Close() directly, instead of using `defer w.Close()`
// Not sure why but defer on its own will generate a file with just 'x'
// And if you combine defer with w.Flush() it will generate a file with the correct content
// However, the file will be corrupted (according to git at least)
// Flush on its own results in the same error as above.
// Why would defer w.Close() not work?
func zlib_compress(str string) []byte {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, flate.BestSpeed)
	if err != nil {
		log.Fatal(err)
	}
	w.Write([]byte(str))
	w.Close()
	return buf.Bytes()
}
func get_sha_str(str string) string {
	hasher := sha1.New()
	str_bytes := []byte(str)
	hasher.Write(str_bytes)
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)
}
func Init() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	git_path := path.Join(root, ".git")
	err = os.Mkdir(git_path, fs.ModePerm)
	if err != nil {
		if os.IsExist(err) {
			fmt.Println(".git already exists, checking for subfolders")
		} else if os.IsPermission(err) {
			log.Fatal(git_path + "Permission denied")
		} else {
			log.Fatal(err)
		}
	}
	folderNames := []string{"objects", "refs"}
	for _, folderName := range folderNames {
		folderPath := path.Join(git_path, folderName)
		err = os.Mkdir(folderPath, fs.ModePerm)
		if err != nil {
			fmt.Println("Folder " + folderName + " already exists")
		}
	}
	fmt.Println("Initialized empty got repository in " + git_path)
}
