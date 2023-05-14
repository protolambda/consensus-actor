package main

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/zrnt/eth2/configs"

	"github.com/protolambda/consensus-actor/fun/era"
)

func readEraFile(f io.ReadSeeker) error {
	// start from end
	groupEnd, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek err: %w", err)
	}

	var buf bytes.Buffer
	for {
		slot, err := era.SeekState(f, groupEnd)
		if err != nil {
			return fmt.Errorf("failed to seek to state: %w", err)
		}
		fmt.Printf("reading group with state at slot %d\n", slot)

		buf.Reset()
		if _, err := era.CopyEntry(f, &buf); err != nil {
			return fmt.Errorf("failed to load state data: %w", err)
		}
		fmt.Printf("state: %d\n", buf.Len())

		if slot != 0 {
			for i := uint64(0); i < era.SlotsPerEra; i++ {
				err := era.SeekBlock(f, i, groupEnd)
				if err == era.ErrNotExist {
					fmt.Printf("block %d does not exist\n", i)
					continue
				}
				if err != nil {
					return fmt.Errorf("failed to seek to block %d: %w", i, err)
				}

				buf.Reset()
				if _, err := era.CopyEntry(f, &buf); err != nil {
					return fmt.Errorf("failed to load block %d data: %w", i, err)
				}
				fmt.Printf("block %d: %d\n", i, buf.Len())
			}
		} else {
			break
		}

		err = era.SeekGroupStart(f, groupEnd)
		if err != nil {
			return fmt.Errorf("failed to seek to group start: %w", err)
		}
		groupEnd, err = era.Tell(f)
		if err != nil {
			return fmt.Errorf("unable to tell current offset: %w", err)
		}
		if groupEnd == 0 {
			break
		}
	}
	return nil
}

func dumpEntries(f io.ReadSeeker) error {
	i := 0
	for {
		typ, l, err := era.ReadHeader(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read header of entry %d: %w", i, err)
		}
		if l > 0 {
			if _, err := f.Seek(int64(l), io.SeekCurrent); err != nil {
				return fmt.Errorf("failed to skip content of entry %d: %w", i, err)
			}
		}
		fmt.Printf("entry %d: type: %x length: %d\n", i, typ, l)
		i++
	}
	return nil
}

//func main() {
//	// era/mainnet-00000-4b363db9.era
//	// era/mainnet-00001-40cf2f3c.era
//	f, err := os.OpenFile("era/mainnet-00001-40cf2f3c.era", os.O_RDONLY, 0)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer f.Close()
//
//	err = readEraFile(f)
//	if err != nil {
//		log.Fatal(err)
//	}
//}

func main() {
	st := era.NewStore()
	if err := st.Load("era"); err != nil {
		log.Fatal(err)
	}
	var state phase0.BeaconState
	if err := st.State(0, configs.Mainnet.Wrap(&state)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("validators: %d\n", len(state.Validators))

	var block phase0.SignedBeaconBlock
	if err := st.Block(1, configs.Mainnet.Wrap(&block)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("block: %v\n", block)
}
