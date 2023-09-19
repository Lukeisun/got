package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
)

func main() {
	command := os.Args[1]
	switch command {
	case "init":
		Init()
	case "commit":
		Commit()
	default:
		log.Fatal("Unknown command")
	}
}
func Commit() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	git_path := path.Join(root, ".git")
	// objects_path := path.Join(git_path, "objects")
	// workspace in book
	workspace_path := "."
	ignore_arr := []string{".git", ".", ".."}
	ignore := make(map[string]struct{})
	for _, s := range ignore_arr {
		ignore[s] = struct{}{}
	}
	var arr []string
	filepath.WalkDir(workspace_path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if _, ok := ignore[d.Name()]; ok {
			return nil
		}
		arr = append(arr, path)
		return nil
	})
	// database in book
	database_path := path.Join(git_path, "objects")
	filesystem := os.DirFS(workspace_path)
	for _, s := range arr {
		file, _ := fs.ReadFile(filesystem, s)
		// make blob
		byte_len := fmt.Sprintf("%d", len(file))
		content := "blob " + byte_len + "\x00" + string(file)
		oid := get_sha_str(content)
		// create dir and file
		dir := path.Join(database_path, oid[:2])
		err := os.Mkdir(dir, fs.ModePerm)
		if err != nil {
			if os.IsExist(err) {
				continue
			} else {
				log.Fatal(err)
			}
		}
		file_path := path.Join(dir, oid[2:])
		f, err := os.Create(file_path)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		compressed := deflate(content, flate.BestSpeed)
		f.Write(compressed)
	}
}

func deflate(str string, level int) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, level)
	w.Write([]byte(str))
	w.Flush()

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
		if err != nil {
			fmt.Println("Folder " + folderName + " already exists")
		}
		os.Mkdir(folderPath, fs.ModePerm)
	}
	fmt.Println("Initialized empty got repository in " + git_path)
}
