package main

// minimal_extract_proofs.go

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	inDir  = "/mydata/samurai/exp1/alchemy_proofs"
	outDir = "/mydata/samurai/exp1/proofs"
)

type fileShape struct {
	AccountProof []string `json:"accountProof"`
	Result       *struct {
		AccountProof []string `json:"accountProof"`
	} `json:"result"`
}

func extractProofs() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Println("mkdir out:", err)
		return
	}
	var ok, fail int

	err := filepath.WalkDir(inDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			fail++
			fmt.Println("read:", path, err)
			return nil
		}

		var f fileShape
		if err := json.Unmarshal(b, &f); err != nil {
			fail++
			fmt.Println("json:", path, err)
			return nil
		}

		nodes := f.AccountProof
		if len(nodes) == 0 && f.Result != nil {
			nodes = f.Result.AccountProof
		}
		if len(nodes) == 0 {
			fail++
			fmt.Println("no accountProof:", path)
			return nil
		}

		outName := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())) + ".proof"
		outPath := filepath.Join(outDir, outName)
		out, err := os.Create(outPath)
		if err != nil {
			fail++
			fmt.Println("create:", outPath, err)
			return nil
		}
		defer out.Close()

		for _, s := range nodes {
			if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
				s = s[2:]
			}
			nb, err := hex.DecodeString(s)
			if err != nil {
				fail++
				fmt.Println("hex:", path, err)
				out.Close()
				os.Remove(outPath)
				return nil
			}
			// 4-byte big-endian length, then bytes
			if err := binary.Write(out, binary.BigEndian, uint32(len(nb))); err != nil {
				fail++
				fmt.Println("write len:", outPath, err)
				out.Close()
				os.Remove(outPath)
				return nil
			}
			if _, err := out.Write(nb); err != nil {
				fail++
				fmt.Println("write bytes:", outPath, err)
				out.Close()
				os.Remove(outPath)
				return nil
			}
		}

		ok++
		if ok%2000 == 0 {
			fmt.Printf("processed %d files (failed %d)\n", ok, fail)
		}
		return nil
	})

	if err != nil {
		fmt.Println("walk:", err)
	}
	fmt.Printf("DONE. success=%d failed=%d\n", ok, fail)
}
