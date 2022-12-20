package tile

import (
	"fmt"

	"github.com/unitoftime/ecs"
	"github.com/unitoftime/flow/phy2"
	"github.com/zyedidia/generic/queue"
)

type ChunkLoader interface {
	LoadChunk(chunkPos ChunkPosition) ([][]Tile, error)
	SaveChunk(chunkmap *Chunkmap, chunkPos ChunkPosition) error
}

type ChunkPosition struct {
	X, Y int32
}

type Chunkmap struct {
	ChunkSize int // TODO - implies only square chunks
	TileSize [2]int // In pixels
	math Math
	chunks map[ChunkPosition]*Tilemap
	loader ChunkLoader
}

func NewChunkmap(chunkSize int, tileSize [2]int, math Math) *Chunkmap {
	return &Chunkmap{
		ChunkSize: chunkSize,
		TileSize: tileSize,
		math: math,
		chunks: make(map[ChunkPosition]*Tilemap),
	}
}

func(c *Chunkmap) WithLoader(loader ChunkLoader) *Chunkmap {
	c.loader = loader
	return c
}

// Returns the worldspace position of a chunk
func (c *Chunkmap) ToPosition(chunkPos ChunkPosition) phy2.Vec2 {
	offX, offY := c.math.Position(int(chunkPos.X), int(chunkPos.Y),
		[2]int{c.TileSize[0]*c.ChunkSize, c.TileSize[1]*c.ChunkSize})

	offset := phy2.Vec2{
		float64(offX),
		float64(offY) - (0.5 * float64(c.ChunkSize) * float64(c.TileSize[1])) + float64(c.TileSize[1]/2),
	}
	return offset
}

func (c *Chunkmap) PositionToChunk(x, y float32) ChunkPosition {
	return c.TileToChunk(c.PositionToTile(x, y))
}

func (c *Chunkmap) TileToChunk(tilePos TilePosition) ChunkPosition {
	if tilePos.X < 0 {
		tilePos.X -= (c.ChunkSize - 1)
	}
	if tilePos.Y < 0 {
		tilePos.Y -= (c.ChunkSize - 1)
	}
	chunkX := tilePos.X / c.ChunkSize
	chunkY := tilePos.Y / c.ChunkSize

	return ChunkPosition{int32(chunkX), int32(chunkY)}
}

// Returns the center tile of a chunk
func (c *Chunkmap) ChunkToTile(chunkPos ChunkPosition) TilePosition {
	tileX := int(chunkPos.X) * c.ChunkSize
	tileY := int(chunkPos.Y) * c.ChunkSize

	tilePos := TilePosition{tileX, tileY}

	return tilePos
}

// TODO - It might be cool to have a function which returns a rectangle of chunks as a list (To automatically cull out sections we don't want)
func (c *Chunkmap) GetAllChunks() []*Tilemap {
	ret := make([]*Tilemap, 0, c.NumChunks())
	for _, chunk := range c.chunks {
		ret = append(ret, chunk)
	}
	return ret
}
func (c *Chunkmap) GetAllChunkPositions() []ChunkPosition {
	ret := make([]ChunkPosition, 0, c.NumChunks())
	for chunkPos := range c.chunks {
		ret = append(ret, chunkPos)
	}
	return ret
}


func (c *Chunkmap) GetChunk(chunkPos ChunkPosition) (*Tilemap, bool) {
	chunk, ok := c.chunks[chunkPos]
	if ok {
		return chunk, true
	}

	// If we couldn't load from map, then load from loader
	if c.loader != nil {
		tiles, err := c.loader.LoadChunk(chunkPos)
		if err != nil {
			return nil, false
		}

		return c.AddChunk(chunkPos, tiles), true
	}

	return nil, false
}

// This generates a chunk based on the passed in expansionLambda
func (c *Chunkmap) GenerateChunk(chunkPos ChunkPosition, expansionLambda func(x, y int) Tile) *Tilemap {
	chunk, ok := c.GetChunk(chunkPos)
	if ok {
		return chunk // Return the chunk and don't create, if the chunk is already made
	}

	tileOffset := c.ChunkToTile(chunkPos)

	tiles := make([][]Tile, c.ChunkSize, c.ChunkSize)
	for x := range tiles {
		tiles[x] = make([]Tile, c.ChunkSize, c.ChunkSize)
		for y := range tiles[x] {
			// fmt.Println(x, y, tileOffset.X, tileOffset.Y)
			tiles[x][y] = expansionLambda(x + tileOffset.X, y + tileOffset.Y)
		}
	}

	return c.AddChunk(chunkPos, tiles)

	// chunk = New(tiles, c.TileSize, c.math)

	// // chunkPos := ChunkToPosition(chunkId)
	// offX, offY := c.math.Position(int(chunkPos.X), int(chunkPos.Y),
	// 	[2]int{c.TileSize[0]*c.ChunkSize, c.TileSize[1]*c.ChunkSize})

	// chunk.Offset.X = float64(offX)
	// chunk.Offset.Y = float64(offY)

	// // Write back
	// c.chunks[chunkPos] = chunk
	// return chunk
}

func (c *Chunkmap) SaveChunk(chunkPos ChunkPosition) error {
	if c.loader == nil {
		return fmt.Errorf("Chunkmap loader is nil")
	}

	// TODO - I feel like I need some way to dump a chunk out of memory. like, SaveCHunk(...), then RemoveFromMemory(...) - OR - PersistChunk (...) which just does both of those

	return c.loader.SaveChunk(c, chunkPos)
}

func (c *Chunkmap) AddChunk(chunkPos ChunkPosition, tiles [][]Tile) *Tilemap {
	chunk := New(tiles, c.TileSize, c.math)

	offX, offY := c.math.Position(int(chunkPos.X), int(chunkPos.Y),
		[2]int{c.TileSize[0]*c.ChunkSize, c.TileSize[1]*c.ChunkSize})

	chunk.Offset.X = float64(offX)
	chunk.Offset.Y = float64(offY)

	// Write back
	c.chunks[chunkPos] = chunk
	return chunk
}

func (c *Chunkmap) NumChunks() int {
	return len(c.chunks)
}

func (c *Chunkmap) GetTile(pos TilePosition) (Tile, bool) {
	chunkPos := c.TileToChunk(pos)
	chunk, ok := c.GetChunk(chunkPos)
	if !ok {
		return Tile{}, false
	}

	tileOffset := c.ChunkToTile(chunkPos)
	localTilePos := TilePosition{pos.X - tileOffset.X, pos.Y - tileOffset.Y}
	// fmt.Println("chunk.Get:", chunkPos, pos, localTilePos)
	return chunk.Get(localTilePos)
}

// Tries to set the tile, returns false if the chunk does not exist
func (c *Chunkmap) SetTile(pos TilePosition, tile Tile) bool {
	chunkPos := c.TileToChunk(pos)
	chunk, ok := c.GetChunk(chunkPos)
	if !ok {
		return false
	}

	tileOffset := c.ChunkToTile(chunkPos)
	localTilePos := TilePosition{pos.X - tileOffset.X, pos.Y - tileOffset.Y}
	// fmt.Println("chunk.Get:", chunkPos, pos, localTilePos)
	ok = chunk.Set(localTilePos, tile)
	if !ok {
		panic("Programmer error")
	}
	return true
}

func (c *Chunkmap) TileToPosition(tilePos TilePosition) (float32, float32) {
	x, y := c.math.Position(tilePos.X, tilePos.Y, c.TileSize)
	return x, y
}

func (c *Chunkmap) PositionToTile(x, y float32) TilePosition {
	tX, tY := c.math.PositionToTile(x, y, c.TileSize)
	return TilePosition{tX, tY}
}

func (c *Chunkmap) GetEdgeNeighbors(x, y int) []TilePosition {
	return []TilePosition{
		TilePosition{x+1, y},
		TilePosition{x-1, y},
		TilePosition{x, y+1},
		TilePosition{x, y-1},
	}
}

func (c *Chunkmap) GetNeighborsAtDistance(x, y int, dist int) []TilePosition {
	distance := make(map[TilePosition]int)

	q := queue.New[TilePosition]()
	q.Enqueue(TilePosition{x, y})

	for !q.Empty() {
		current := q.Dequeue()

		d := distance[current]
		if d >= dist { continue } // Don't need to search past our limit

		neighbors := c.GetEdgeNeighbors(current.X, current.Y)
		for _, next := range neighbors {
			_, ok := c.GetTile(next)
			if !ok { continue } // Skip as neighbor doesn't actually exist (ie could be OOB)

			// If we haven't already walked over this neighbor, then enqueue it and add it to our path
			_, exists := distance[next]
			if !exists {
				q.Enqueue(next)
				distance[next] = 1 + distance[current]
			}
		}
	}

	// Pull out all of the tiles that are at the correct distance
	ret := make([]TilePosition, 0)
	for pos, d := range distance {
		if d != dist { continue } // Don't return if distance isn't corect
		ret = append(ret, pos)
	}
	return ret
}

func (c *Chunkmap) GetChunkEdgeNeighbors(pos ChunkPosition) []ChunkPosition {
	return []ChunkPosition{
		ChunkPosition{pos.X+1, pos.Y},
		ChunkPosition{pos.X-1, pos.Y},
		ChunkPosition{pos.X, pos.Y+1},
		ChunkPosition{pos.X, pos.Y-1},
	}
}

func (c *Chunkmap) GetPerimeter() map[ChunkPosition]bool {
	perimeter := make(map[ChunkPosition]bool) // List of chunkPositions that are the perimeter
	processed := make(map[ChunkPosition]bool) // List of chunkPositions that we've already processed

	// Just start at some random chunkPosition (whichever is first
	var start ChunkPosition
	for chunkPos := range c.chunks {
		start = chunkPos
		break
	}

	q := queue.New[ChunkPosition]()
	q.Enqueue(start)

	for !q.Empty() {
		current := q.Dequeue()

		neighbors := c.GetChunkEdgeNeighbors(current)
		for _, next := range neighbors {
			_, ok := c.GetChunk(next)
			if ok {
				// If the chunk's neighbor exists, then add it and keep processing
				_, alreadyProcessed := processed[next]
				if !alreadyProcessed {
					q.Enqueue(next)
					processed[next] = true
				}
				continue
			}

			perimeter[next] = true
		}
	}

	return perimeter
}

// TODO - optimize in a locality sort of way
// Recalculates all of the entities that are on tiles based on tile colliders
func (c *Chunkmap) RecalculateEntities(world *ecs.World) {
	for _, chunk := range c.chunks {
		// Clear Entities
		for x := range chunk.tiles {
			for y := range chunk.tiles[x] {
				chunk.tiles[x][y].Entity = ecs.InvalidEntity
			}
		}
	}

	// Recompute all entities with TileColliders
	ecs.Map2(world, func(id ecs.Id, collider *Collider, pos *phy2.Pos) {
		tilePos := c.PositionToTile(float32(pos.X), float32(pos.Y))

		for x := tilePos.X; x < tilePos.X + collider.Width; x++ {
			for y := tilePos.Y; y < tilePos.Y + collider.Height; y++ {
				chunkPos := c.TileToChunk(TilePosition{x, y})
				chunk, ok := c.GetChunk(chunkPos)
				if !ok { panic("Something has been built on a chunk that doesn't exist!") }
				localTilePosition := chunk.PositionToTile(float32(pos.X), float32(pos.Y))
				chunk.tiles[localTilePosition.X][localTilePosition.Y].Entity = id // Store the entity
			}
		}
	})
}

// Returns a list of tiles that are overlapping the collider at a position
func (t *Chunkmap) GetOverlappingTiles(x, y float64, collider *phy2.CircleCollider) []TilePosition {
	minX := x - collider.Radius
	maxX := x + collider.Radius
	minY := y - collider.Radius
	maxY := y + collider.Radius

	min := t.PositionToTile(float32(minX), float32(minY))
	max := t.PositionToTile(float32(maxX), float32(maxY))

	ret := make([]TilePosition, 0)
	for tx := min.X; tx <= max.X; tx++ {
		for ty := min.Y; ty <= max.Y; ty++ {
			ret = append(ret, TilePosition{tx, ty})
		}
	}
	return ret
}
