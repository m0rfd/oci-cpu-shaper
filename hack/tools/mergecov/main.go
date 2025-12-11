package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"

	"golang.org/x/tools/cover"
)

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatal("at least one coverage profile is required")
	}

	merged := make(map[string]*cover.Profile)

	for _, path := range flag.Args() {
		profiles, err := cover.ParseProfiles(path)
		if err != nil {
			log.Fatalf("parse profile %s: %v", path, err)
		}

		for _, profile := range profiles {
			existingProfile, ok := merged[profile.FileName]
			if !ok {
				merged[profile.FileName] = &cover.Profile{ //nolint:exhaustruct // only required fields
					Mode:     profile.Mode,
					FileName: profile.FileName,
				}
				existingProfile = merged[profile.FileName]
			}

			if existingProfile.Mode == "" {
				existingProfile.Mode = profile.Mode
			}

			if existingProfile.Mode != profile.Mode {
				log.Fatalf(
					"profile mode mismatch for %s: %s vs %s",
					profile.FileName,
					existingProfile.Mode,
					profile.Mode,
				)
			}

			existingProfile.Blocks = appendBlocks(existingProfile.Blocks, profile.Blocks)
		}
	}

	profiles := make([]*cover.Profile, 0, len(merged))
	for _, profile := range merged {
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool { return profiles[i].FileName < profiles[j].FileName })

	err := writeProfiles(profiles, os.Stdout)
	if err != nil {
		log.Fatalf("write merged profile: %v", err)
	}
}

func appendBlocks(dest, src []cover.ProfileBlock) []cover.ProfileBlock {
	for _, block := range src {
		replaced := false

		for idx, existing := range dest {
			if sameBlock(existing, block) {
				dest[idx].Count += block.Count
				replaced = true

				break
			}

			if overlap(existing, block) {
				if block.Count > existing.Count {
					dest[idx].Count = block.Count
				}

				replaced = true

				break
			}
		}

		if !replaced {
			dest = append(dest, block)
		}
	}

	return dest
}

func sameBlock(first, second cover.ProfileBlock) bool {
	return first.StartLine == second.StartLine && first.StartCol == second.StartCol &&
		first.EndLine == second.EndLine && first.EndCol == second.EndCol
}

func overlap(blockA, blockB cover.ProfileBlock) bool {
	if blockA.EndLine < blockB.StartLine || blockB.EndLine < blockA.StartLine {
		return false
	}

	if blockA.EndLine == blockB.StartLine && blockA.EndCol <= blockB.StartCol {
		return false
	}

	if blockB.EndLine == blockA.StartLine && blockB.EndCol <= blockA.StartCol {
		return false
	}

	return true
}

func writeProfiles(profiles []*cover.Profile, writer io.Writer) error {
	if len(profiles) == 0 {
		return nil
	}

	mode := profiles[0].Mode

	_, err := fmt.Fprintf(writer, "mode: %s\n", mode)
	if err != nil {
		return fmt.Errorf("write mode: %w", err)
	}

	for _, profile := range profiles {
		for _, block := range profile.Blocks {
			_, err = fmt.Fprintf(
				writer,
				"%s:%d.%d,%d.%d %d %d\n",
				profile.FileName,
				block.StartLine,
				block.StartCol,
				block.EndLine,
				block.EndCol,
				block.NumStmt,
				block.Count,
			)
			if err != nil {
				return fmt.Errorf("write block: %w", err)
			}
		}
	}

	return nil
}
