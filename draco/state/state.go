package state

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"sort"
	"strings"
	"unsafe"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity/physics"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
)

var (
	//go:embed 1.18.10.nbt
	blockStateData []byte
	// blocks holds a list of all registered Blocks indexed by their runtime ID. Blocks that were not explicitly
	// registered are of the type unknownBlock.
	blocks []world.Block
	// stateRuntimeIDs holds a map for looking up the runtime ID of a block by the stateHash it produces.
	stateRuntimeIDs = map[stateHash]uint32{}
	// nbtBlocks holds a list of NBTer implementations for blocks registered that implement the NBTer interface.
	// These are indexed by their runtime IDs. Blocks that do not implement NBTer have a false value in this slice.
	nbtBlocks []bool
	// randomTickBlocks holds a list of RandomTicker implementations for blocks registered that implement the RandomTicker interface.
	// These are indexed by their runtime IDs. Blocks that do not implement RandomTicker have a false value in this slice.
	randomTickBlocks []bool
	// airRID is the runtime ID of an air block.
	airRID uint32
)

var StateToRuntimeID func(string, map[string]interface{}) (uint32, bool)

func init() {
	dec := nbt.NewDecoder(bytes.NewBuffer(blockStateData))

	// Register all block states present in the block_states.nbt file. These are all possible options registered
	// blocks may encode to.
	var s blockState
	for {
		if err := dec.Decode(&s); err != nil {
			break
		}
		registerBlockState(s)
	}

	StateToRuntimeID = func(name string, properties map[string]interface{}) (runtimeID uint32, found bool) {
		rid, ok := stateRuntimeIDs[stateHash{name: name, properties: hashProperties(properties)}]
		return rid, ok
	}
}

// registerBlockState registers a new blockState to the states slice. The function panics if the properties the
// blockState hold are invalid or if the blockState was already registered.
func registerBlockState(s blockState) {
	h := stateHash{name: s.Name, properties: hashProperties(s.Properties)}
	if _, ok := stateRuntimeIDs[h]; ok {
		panic(fmt.Sprintf("cannot register the same state twice (%+v)", s))
	}
	rid := uint32(len(blocks))
	if s.Name == "minecraft:air" {
		airRID = rid
	}
	stateRuntimeIDs[h] = rid
	blocks = append(blocks, unknownBlock{s})

	nbtBlocks = append(nbtBlocks, false)
	randomTickBlocks = append(randomTickBlocks, false)
	chunk.FilteringBlocks = append(chunk.FilteringBlocks, 15)
	chunk.LightBlocks = append(chunk.LightBlocks, 0)
}

// unknownModel is the model used for unknown blocks. It is the equivalent of a fully solid model.
type unknownModel struct{}

// AABB ...
func (u unknownModel) AABB(cube.Pos, *world.World) []physics.AABB {
	return []physics.AABB{physics.NewAABB(mgl64.Vec3{}, mgl64.Vec3{1, 1, 1})}
}

// FaceSolid ...
func (u unknownModel) FaceSolid(cube.Pos, cube.Face, *world.World) bool {
	return true
}

// unknownBlock represents a block that has not yet been implemented. It is used for registering block
// states that haven't yet been added.
type unknownBlock struct {
	blockState
}

// EncodeBlock ...
func (b unknownBlock) EncodeBlock() (string, map[string]interface{}) {
	return b.Name, b.Properties
}

// Model ...
func (unknownBlock) Model() world.BlockModel {
	return unknownModel{}
}

// Hash ...
func (b unknownBlock) Hash() uint64 {
	return math.MaxUint64
}

// blockState holds a combination of a name and properties, together with a version.
type blockState struct {
	Name       string                 `nbt:"name"`
	Properties map[string]interface{} `nbt:"states"`
	Version    int32                  `nbt:"version"`
}

// stateHash is a struct that may be used as a map key for block states. It contains the name of the block state
// and an encoded version of the properties.
type stateHash struct {
	name, properties string
}

// HashProperties produces a hash for the block properties held by the blockState.
func hashProperties(properties map[string]interface{}) string {
	if properties == nil {
		return ""
	}
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	var b strings.Builder
	for _, k := range keys {
		switch v := properties[k].(type) {
		case bool:
			if v {
				b.WriteByte(1)
			} else {
				b.WriteByte(0)
			}
		case uint8:
			b.WriteByte(v)
		case int32:
			a := *(*[4]byte)(unsafe.Pointer(&v))
			b.Write(a[:])
		case string:
			b.WriteString(v)
		default:
			// If block encoding is broken, we want to find out as soon as possible. This saves a lot of time
			// debugging in-game.
			panic(fmt.Sprintf("invalid block property type %T for property %v", v, k))
		}
	}

	return b.String()
}