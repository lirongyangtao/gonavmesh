package recast

import (
	"gonavamesh/common"
	"math"
)

type DtObstacleRef int

type DtCompressedTileRef int

/// Flags for addTile

const DT_COMPRESSEDTILE_FREE_DATA = 0x01 ///< Navmesh owns the tile memory and should free it.

type dtCompressedTile struct {
	salt           int ///< Counter describing modifications to the tile.
	Header         *DtTileCacheLayerHeader
	compressed     []int
	compressedSize int
	data           []byte
	dataSize       int
	flags          int
	next           *dtCompressedTile
}

const (
	DT_OBSTACLE_EMPTY = iota
	DT_OBSTACLE_PROCESSING
	DT_OBSTACLE_PROCESSED
	DT_OBSTACLE_REMOVING
)

const (
	DT_OBSTACLE_CYLINDER     = iota
	DT_OBSTACLE_BOX          // AABB
	DT_OBSTACLE_ORIENTED_BOX // OBB
)

type dtObstacleCylinder struct {
	pos    [3]float64
	radius float64
	height float64
}

type dtObstacleBox struct {
	bmin [3]float64
	bmax [3]float64
}

type dtObstacleOrientedBox struct {
	center      [3]float64
	halfExtents [3]float64
	rotAux      [2]float64 //{ cos(0.5f*angle)*sin(-0.5f*angle); cos(0.5f*angle)*cos(0.5f*angle) - 0.5 }
}

type DtTileCacheParams struct {
	Orig                   [3]float64
	Cs, Ch                 float64
	Width, Height          int
	WalkableHeight         float64
	WalkableRadius         float64
	WalkableClimb          float64
	MaxSimplificationError float64
	MaxTiles               int
	MaxObstacles           int
}

const (
	TileCache_MAX_UPDATE   = 64
	TileCache_MAX_REQUESTS = 64
)

type DtTileCache struct {
	m_tileLutSize int ///< Tile hash lookup size (must be pot).
	m_tileLutMask int ///< Tile hash lookup mask.

	m_posLookup    []*dtCompressedTile ///< Tile hash lookup.
	m_nextFreeTile *dtCompressedTile   ///< Freelist of tiles.
	m_tiles        []*dtCompressedTile ///< List of tiles.

	m_saltBits int ///< Number of salt bits in the tile ID.
	m_tileBits int ///< Number of tile bits in the tile ID.

	m_params *DtTileCacheParams
	m_tcomp  *DtTileCacheCompressor
	m_tmproc DtTileCacheMeshProcess

	m_obstacles        []*DtTileCacheObstacle
	m_nextFreeObstacle *DtTileCacheObstacle
	m_reqs             [TileCache_MAX_REQUESTS]*ObstacleRequest
	m_nreqs            int

	m_update  [TileCache_MAX_UPDATE]DtCompressedTileRef
	m_nupdate int
}

func (d *DtTileCache) GetCompressor() *DtTileCacheCompressor { return d.m_tcomp }
func (d *DtTileCache) GetParams() *DtTileCacheParams         { return d.m_params }

func (d *DtTileCache) GetTileCount() int               { return d.m_params.MaxTiles }
func (d *DtTileCache) GetTile(i int) *dtCompressedTile { return d.m_tiles[i] }

func (d *DtTileCache) GetObstacleCount() int                  { return d.m_params.MaxObstacles }
func (d *DtTileCache) GetObstacle(i int) *DtTileCacheObstacle { return d.m_obstacles[i] }
func (d *DtTileCache) getTileIndex(t *dtCompressedTile) int {
	for i, v := range d.m_tiles {
		if v == t {
			return i
		}
	}
	return -1
}

func (d *DtTileCache) getObstaclesIndex(o *DtTileCacheObstacle) int {
	for i, v := range d.m_obstacles {
		if v == o {
			return i
		}
	}
	return -1
}

// / Encodes a tile id.
func (d *DtTileCache) encodeTileId(salt int, it int) DtCompressedTileRef {
	return DtCompressedTileRef((salt << d.m_tileBits) | it)
}

// / Decodes a tile salt.
func (d *DtTileCache) decodeTileIdSalt(ref DtCompressedTileRef) int {
	saltMask := (1 << d.m_saltBits) - 1
	return (int(ref) >> d.m_tileBits) & saltMask
}

// / Decodes a tile id.
func (d *DtTileCache) decodeTileIdTile(ref DtCompressedTileRef) int {
	tileMask := (1 << d.m_tileBits) - 1
	return int(ref) & tileMask
}

// / Encodes an obstacle id.
func (d *DtTileCache) encodeObstacleId(salt int, it int) DtObstacleRef {
	return DtObstacleRef(salt<<16 | it)
}

// / Decodes an obstacle salt.
func (d *DtTileCache) decodeObstacleIdSalt(ref DtObstacleRef) int {
	saltMask := (1 << 16) - 1
	return ((int(ref) >> 16) & saltMask)
}

// / Decodes an obstacle id.
func (d *DtTileCache) decodeObstacleIdObstacle(ref DtObstacleRef) int {
	tileMask := (1 << 16) - 1
	return int(ref) & tileMask
}

const (
	REQUEST_ADD = iota
	REQUEST_REMOVE
)
const (
	MAX_REQUESTS = 64
	MAX_UPDATE   = 64
)
const DT_MAX_TOUCHED_TILES = 8

type DtTileCacheObstacle struct {
	cylinder    dtObstacleCylinder
	box         dtObstacleBox
	orientedBox dtObstacleOrientedBox
	touched     [DT_MAX_TOUCHED_TILES]DtCompressedTileRef
	pending     [DT_MAX_TOUCHED_TILES]DtCompressedTileRef
	salt        int
	Type        int
	State       int
	ntouched    int
	npending    int
	next        *DtTileCacheObstacle
}

type ObstacleRequest struct {
	action int
	ref    DtObstacleRef

	m_tileLutSize int ///< Tile hash lookup size (must be pot).
	m_tileLutMask int ///< Tile hash lookup mask.

	m_posLookup    []*dtCompressedTile ///< Tile hash lookup.
	m_nextFreeTile *dtCompressedTile   ///< Freelist of tiles.
	m_tiles        *dtCompressedTile   ///< List of tiles.

	m_saltBits int ///< Number of salt bits in the tile ID.
	m_tileBits int ///< Number of tile bits in the tile ID.

	m_params           *DtTileCacheParams
	m_tcomp            *DtTileCacheCompressor
	m_tmproc           *DtTileCacheMeshProcess
	m_obstacles        *DtTileCacheObstacle
	m_nextFreeObstacle *DtTileCacheObstacle
	m_reqs             [MAX_REQUESTS]*ObstacleRequest
	m_nreqs            int

	m_update  [MAX_UPDATE]DtCompressedTileRef
	m_nupdate int
}

func newObstacleRequest() *ObstacleRequest {
	return &ObstacleRequest{}
}
func titleCacheContains(a []DtCompressedTileRef, n int, v DtCompressedTileRef) bool {
	for i := 0; i < n; i++ {
		if a[i] == v {
			return true
		}
	}
	return false
}

func titleCacheComputeTileHash(x int, y int, mask int) int {
	h1 := 0x8da6b343 // Large multiplicative constants;
	h2 := 0xd8163841 // here arbitrarily chosen primes
	n := h1*x + h2*y
	return (n & mask)
}

type NavMeshTileBuildContext struct {
	layer *DtTileCacheLayer
	lcset *DtTileCacheContourSet
	lmesh *DtTileCachePolyMesh
}

func (d *NavMeshTileBuildContext) purge() {
	d.layer = nil
	d.lcset = nil
	d.lmesh = nil
}

func (d *DtTileCache) GetTileByRef(ref DtCompressedTileRef) *dtCompressedTile {
	if ref == 0 {
		return nil
	}

	tileIndex := d.decodeTileIdTile(ref)
	tileSalt := d.decodeTileIdSalt(ref)
	if tileIndex >= d.m_params.MaxTiles {
		return nil
	}
	tile := d.m_tiles[tileIndex]
	if tile.salt != tileSalt {
		return nil
	}

	return tile
}

func (d *DtTileCache) init(params *DtTileCacheParams,
	tcomp *DtTileCacheCompressor,
	tmproc DtTileCacheMeshProcess) DtStatus {
	d.m_tcomp = tcomp
	d.m_tmproc = tmproc
	d.m_nreqs = 0
	d.m_params = params

	// Alloc space for obstacles.
	d.m_obstacles = make([]*DtTileCacheObstacle, d.m_params.maxObstacles)
	for i := range d.m_obstacles {
		d.m_obstacles[i] = &DtTileCacheObstacle{}
	}
	d.m_nextFreeObstacle = nil
	for i := d.m_params.maxObstacles - 1; i >= 0; i-- {
		d.m_obstacles[i].salt = 1
		d.m_obstacles[i].next = d.m_nextFreeObstacle
		d.m_nextFreeObstacle = d.m_obstacles[i]
	}

	// Init tiles
	d.m_tileLutSize = common.NextPow2(d.m_params.maxTiles / 4)
	if d.m_tileLutSize == 0 {
		d.m_tileLutSize = 1
	}
	d.m_tileLutMask = d.m_tileLutSize - 1

	d.m_tiles = make([]*dtCompressedTile, d.m_params.maxTiles)
	for i := range d.m_tiles {
		d.m_tiles[i] = &dtCompressedTile{}
	}

	d.m_posLookup = make([]*dtCompressedTile, d.m_tileLutSize)
	for i := range d.m_posLookup {
		d.m_posLookup[i] = &dtCompressedTile{}
	}
	d.m_nextFreeTile = nil
	for i := d.m_params.maxTiles - 1; i >= 0; i-- {
		d.m_tiles[i].salt = 1
		d.m_tiles[i].next = d.m_nextFreeTile
		d.m_nextFreeTile = d.m_tiles[i]
	}

	// Init ID generator values.
	d.m_tileBits = common.Ilog2(common.NextPow2(d.m_params.maxTiles))
	// Only allow 31 salt bits, since the salt mask is calculated using 32bit uint and it will overflow.
	d.m_saltBits = common.Min(31, 32-d.m_tileBits)
	if d.m_saltBits < 10 {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	return DT_SUCCESS
}

func (d *DtTileCache) GetTilesAt(tx, ty int, tiles []DtCompressedTileRef, maxTiles int) int {
	n := 0

	// Find tile based on hash.
	h := computeTileHash(tx, ty, d.m_tileLutMask)
	tile := d.m_posLookup[h]
	for tile != nil {
		if tile.header != nil &&
			tile.header.Tx == tx &&
			tile.header.Ty == ty {
			if n < maxTiles {
				tiles[n] = d.getTileRef(tile)
				n++
			}

		}
		tile = tile.next
	}

	return n
}

func (d *DtTileCache) getTileAt(tx, ty, tlayer int) *dtCompressedTile {
	// Find tile based on hash.
	h := computeTileHash(tx, ty, d.m_tileLutMask)
	tile := d.m_posLookup[h]
	for tile != nil {
		if tile.header != nil &&
			tile.header.Tx == tx &&
			tile.header.Ty == ty &&
			tile.header.Tlayer == tlayer {
			return tile
		}
		tile = tile.next
	}
	return nil
}

func (d *DtTileCache) getTileRef(tile *dtCompressedTile) DtCompressedTileRef {
	if tile == nil {
		return 0
	}
	it := d.getTileIndex(tile)
	return d.encodeTileId(tile.salt, it)
}

func (d *DtTileCache) GetObstacleRef(ob *DtTileCacheObstacle) DtObstacleRef {
	if ob == nil {
		return 0
	}
	idx := d.getObstaclesIndex(ob)
	return d.encodeObstacleId(ob.salt, idx)
}

func (d *DtTileCache) getObstacleByRef(ref DtObstacleRef) *DtTileCacheObstacle {
	if ref == 0 {
		return nil
	}

	idx := d.decodeObstacleIdObstacle(ref)
	if idx >= d.m_params.maxObstacles {
		return nil
	}

	ob := d.m_obstacles[idx]
	salt := d.decodeObstacleIdSalt(ref)
	if ob.salt != salt {
		return nil
	}

	return ob
}

func (d *DtTileCache) AddTile(header *DtTileCacheLayerHeader, tile *dtCompressedTile, flags int, result *DtCompressedTileRef) DtStatus {
	// Make sure the data is in right format.
	if header.Magic != DT_TILECACHE_MAGIC {
		return DT_FAILURE | DT_WRONG_MAGIC
	}

	if header.Version != DT_TILECACHE_VERSION {
		return DT_FAILURE | DT_WRONG_VERSION
	}

	// Make sure the location is free.
	if d.getTileAt(header.Tx, header.Ty, header.Tlayer) != nil {
		return DT_FAILURE
	}

	// Allocate a tile.
	if d.m_nextFreeTile != nil {
		tile = d.m_nextFreeTile
		d.m_nextFreeTile = tile.next
		tile.next = nil
	}

	// Make sure we could allocate a tile.
	if tile == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	// Insert tile into the position lut.
	h := computeTileHash(header.Tx, header.Ty, d.m_tileLutMask)
	tile.next = d.m_posLookup[h]
	d.m_posLookup[h] = tile

	// Init tile.
	tile.flags = flags

	if result != nil {
		*result = d.getTileRef(tile)
	}

	return DT_SUCCESS
}

func (d *DtTileCache) removeTile(ref DtCompressedTileRef, data *[]byte, dataSize *int) DtStatus {
	if ref == 0 {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	tileIndex := d.decodeTileIdTile(ref)
	tileSalt := d.decodeTileIdSalt(ref)
	if tileIndex >= d.m_params.maxTiles {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	tile := d.m_tiles[tileIndex]
	if tile.salt != tileSalt {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	// Remove tile from hash lookup.
	h := computeTileHash(tile.header.Tx, tile.header.Ty, d.m_tileLutMask)
	var prev *dtCompressedTile
	cur := d.m_posLookup[h]
	for cur != nil {
		if cur == tile {
			if prev != nil {
				prev.next = cur.next
			} else {
				d.m_posLookup[h] = cur.next
			}

			break
		}
		prev = cur
		cur = cur.next
	}

	// Reset tile.
	if tile.flags&DT_COMPRESSEDTILE_FREE_DATA > 0 {
		// Owns data
		tile.data = tile.data[:0]
		tile.dataSize = 0
		if *data != nil {
			*data = nil
		}
		if dataSize != nil {
			*dataSize = 0
		}
	} else {
		if *data != nil {
			*data = tile.data
		}
		if dataSize != nil {
			*dataSize = tile.dataSize
		}
	}

	tile.header = nil
	tile.data = nil
	tile.dataSize = 0
	tile.compressed = nil
	tile.compressedSize = 0
	tile.flags = 0

	// Update salt, salt should never be zero.
	tile.salt = (tile.salt + 1) & ((1 << d.m_saltBits) - 1)
	if tile.salt == 0 {
		tile.salt++
	}

	// Add to free list.
	tile.next = d.m_nextFreeTile
	d.m_nextFreeTile = tile

	return DT_SUCCESS
}

func (d *DtTileCache) AddObstacle(pos []float64, radius, height float64, result *DtObstacleRef) DtStatus {
	if d.m_nreqs >= MAX_REQUESTS {
		return DT_FAILURE | DT_BUFFER_TOO_SMALL
	}

	var ob *DtTileCacheObstacle
	if d.m_nextFreeObstacle != nil {
		ob = d.m_nextFreeObstacle
		d.m_nextFreeObstacle = ob.next
		ob.next = nil
	}
	if ob == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	salt := ob.salt
	ob = &DtTileCacheObstacle{}
	ob.salt = salt
	ob.State = DT_OBSTACLE_PROCESSING
	ob.Type = DT_OBSTACLE_CYLINDER
	copy(ob.cylinder.pos[:], pos)
	ob.cylinder.radius = radius
	ob.cylinder.height = height

	req := &ObstacleRequest{}
	d.m_nreqs++
	d.m_reqs[d.m_nreqs] = req
	req.action = REQUEST_ADD
	req.ref = d.GetObstacleRef(ob)

	if result != nil {
		*result = req.ref
	}

	return DT_SUCCESS
}
func (d *DtTileCache) addBoxObstacle(bmin, bmax []float64, result *DtObstacleRef) DtStatus {
	if d.m_nreqs >= MAX_REQUESTS {
		return DT_FAILURE | DT_BUFFER_TOO_SMALL
	}

	var ob *DtTileCacheObstacle
	if d.m_nextFreeObstacle != nil {
		ob = d.m_nextFreeObstacle
		d.m_nextFreeObstacle = ob.next
		ob.next = nil
	}
	if ob == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	salt := ob.salt
	ob = &DtTileCacheObstacle{}
	ob.salt = salt
	ob.State = DT_OBSTACLE_PROCESSING
	ob.Type = DT_OBSTACLE_BOX
	copy(ob.box.bmin[:], bmin)
	copy(ob.box.bmax[:], bmax)

	req := &ObstacleRequest{}
	d.m_reqs[d.m_nreqs] = req
	d.m_nreqs++
	req.action = REQUEST_ADD
	req.ref = d.GetObstacleRef(ob)

	if result != nil {
		*result = req.ref
	}

	return DT_SUCCESS
}

func (d *DtTileCache) addBoxObstacle1(center []float64, halfExtents []float64, yRadians float64, result *DtObstacleRef) DtStatus {
	if d.m_nreqs >= MAX_REQUESTS {
		return DT_FAILURE | DT_BUFFER_TOO_SMALL
	}

	var ob *DtTileCacheObstacle
	if d.m_nextFreeObstacle != nil {
		ob = d.m_nextFreeObstacle
		d.m_nextFreeObstacle = ob.next
		ob.next = nil
	}
	if ob == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	salt := ob.salt
	ob = &DtTileCacheObstacle{}
	ob.salt = salt
	ob.State = DT_OBSTACLE_PROCESSING
	ob.Type = DT_OBSTACLE_ORIENTED_BOX
	copy(ob.orientedBox.center[:], center)
	copy(ob.orientedBox.halfExtents[:], halfExtents)

	coshalf := math.Cos(0.5 * yRadians)
	sinhalf := math.Sin(-0.5 * yRadians)
	ob.orientedBox.rotAux[0] = coshalf * sinhalf
	ob.orientedBox.rotAux[1] = coshalf*coshalf - 0.5

	req := &ObstacleRequest{}
	d.m_reqs[d.m_nreqs] = req
	d.m_nreqs++
	req.action = REQUEST_ADD
	req.ref = d.GetObstacleRef(ob)

	if result != nil {
		*result = req.ref
	}

	return DT_SUCCESS
}

func (d *DtTileCache) queryTiles(bmin, bmax []float64,
	results []DtCompressedTileRef, resultCount *int, maxResults int) DtStatus {
	MAX_TILES := 32
	tiles := make([]DtCompressedTileRef, MAX_TILES)

	n := 0

	tw := float64(d.m_params.Width) * d.m_params.Cs
	th := float64(d.m_params.Height) * d.m_params.Cs
	tx0 := int(math.Floor((bmin[0] - d.m_params.Orig[0]) / tw))
	tx1 := int(math.Floor((bmax[0] - d.m_params.Orig[0]) / tw))
	ty0 := int(math.Floor((bmin[2] - d.m_params.Orig[2]) / th))
	ty1 := int(math.Floor((bmax[2] - d.m_params.Orig[2]) / th))

	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			ntiles := d.GetTilesAt(tx, ty, tiles, MAX_TILES)

			for i := 0; i < ntiles; i++ {
				tile := d.m_tiles[d.decodeTileIdTile(tiles[i])]
				tbmin := make([]float64, 3)
				tbmax := make([]float64, 3)
				d.CalcTightTileBounds(tile.Header, tbmin, tbmax)

				if dtOverlapBounds(bmin, bmax, tbmin, tbmax) {
					if n < maxResults {
						results[n] = tiles[i]
						n++
					}

				}
			}
		}
	}

	*resultCount = n

	return DT_SUCCESS
}

func (d *DtTileCache) Update(dt float64, navmesh IDtNavMesh,
	upToDates ...*bool) DtStatus {
	var upToDate *bool
	if len(upToDates) > 0 {
		upToDate = upToDates[0]
	}
	if d.m_nupdate == 0 {
		// Process requests.
		for i := 0; i < d.m_nreqs; i++ {
			req := d.m_reqs[i]

			idx := d.decodeObstacleIdObstacle(req.ref)
			if idx >= d.m_params.MaxObstacles {
				continue
			}

			ob := d.m_obstacles[idx]
			salt := d.decodeObstacleIdSalt(req.ref)
			if ob.salt != salt {
				continue
			}

			if req.action == REQUEST_ADD {
				// Find touched tiles.
				bmin := make([]float64, 3)
				bmax := make([]float64, 3)
				d.GetObstacleBounds(ob, bmin, bmax)

				ntouched := 0
				d.queryTiles(bmin, bmax, ob.touched[:], &ntouched, DT_MAX_TOUCHED_TILES)
				ob.ntouched = ntouched
				// Add tiles to update list.
				ob.npending = 0
				for j := 0; j < ob.ntouched; j++ {
					if d.m_nupdate < MAX_UPDATE {
						if !titleCacheContains(d.m_update[:], d.m_nupdate, ob.touched[j]) {
							d.m_update[d.m_nupdate] = ob.touched[j]
						}

						d.m_nupdate++
						ob.pending[ob.npending] = ob.touched[j]
						ob.npending++
					}
				}
			} else if req.action == REQUEST_REMOVE {
				// Prepare to remove obstacle.
				ob.State = DT_OBSTACLE_REMOVING
				// Add tiles to update list.
				ob.npending = 0
				for j := 0; j < ob.ntouched; j++ {
					if d.m_nupdate < MAX_UPDATE {
						if !titleCacheContains(d.m_update[:], d.m_nupdate, ob.touched[j]) {
							d.m_update[d.m_nupdate] = ob.touched[j]
						}

						d.m_nupdate++
						ob.pending[ob.npending] = ob.touched[j]
						ob.npending++
					}
				}
			}
		}

		d.m_nreqs = 0
	}

	var status DtStatus = DT_SUCCESS
	// Process updates
	if d.m_nupdate != 0 {
		// Build mesh
		ref := d.m_update[0]
		status = d.buildNavMeshTile(ref, navmesh)
		d.m_nupdate--
		if d.m_nupdate > 0 {
			copy(d.m_update[:], d.m_update[1:1+d.m_nupdate])

		}

		// Update obstacle states.
		for i := 0; i < d.m_params.MaxObstacles; i++ {
			ob := d.m_obstacles[i]
			if ob.State == DT_OBSTACLE_PROCESSING || ob.State == DT_OBSTACLE_REMOVING {
				// Remove handled tile from pending list.
				for j := 0; j < ob.npending; j++ {
					if ob.pending[j] == ref {
						ob.pending[j] = ob.pending[ob.npending-1]
						ob.npending--
						break
					}
				}

				// If all pending tiles processed, change state.
				if ob.npending == 0 {
					if ob.State == DT_OBSTACLE_PROCESSING {
						ob.State = DT_OBSTACLE_PROCESSED
					} else if ob.State == DT_OBSTACLE_REMOVING {
						ob.State = DT_OBSTACLE_EMPTY
						// Update salt, salt should never be zero.
						ob.salt = (ob.salt + 1) & ((1 << 16) - 1)
						if ob.salt == 0 {
							ob.salt++
						}

						// Return obstacle to free list.
						ob.next = d.m_nextFreeObstacle
						d.m_nextFreeObstacle = ob
					}
				}
			}
		}
	}

	if upToDate != nil {
		*upToDate = d.m_nupdate == 0 && d.m_nreqs == 0
	}

	return status
}

func (d *DtTileCache) BuildNavMeshTilesAt(tx, ty int, navmesh *DtNavMesh) DtStatus {
	MAX_TILES := 32
	tiles := make([]DtCompressedTileRef, MAX_TILES)
	ntiles := d.GetTilesAt(tx, ty, tiles, MAX_TILES)

	for i := 0; i < ntiles; i++ {
		status := d.buildNavMeshTile(tiles[i], navmesh)
		if status.DtStatusFailed() {
			return status
		}

	}

	return DT_SUCCESS
}

func (d *DtTileCache) buildNavMeshTile(ref DtCompressedTileRef, navmesh *DtNavMesh) DtStatus {
	idx := d.decodeTileIdTile(ref)
	if idx > d.m_params.maxTiles {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	tile := d.m_tiles[idx]
	salt := d.decodeTileIdSalt(ref)
	if tile.salt != salt {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	bc := &NavMeshTileBuildContext{}
	walkableClimbVx := int(d.m_params.walkableClimb / d.m_params.ch)
	var status DtStatus

	// Decompress tile layer data.
	bc.layer, status = dtDecompressTileCacheLayer(d.m_tcomp, tile.data, tile.dataSize)
	if status.DtStatusFailed() {
		return status
	}

	// Rasterize obstacles.
	for i := 0; i < d.m_params.maxObstacles; i++ {
		ob := d.m_obstacles[i]
		if ob.state == DT_OBSTACLE_EMPTY || ob.state == DT_OBSTACLE_REMOVING {
			continue
		}

		if titleCacheContains(ob.touched[:], ob.ntouched, ref) {
			if ob.Type == DT_OBSTACLE_CYLINDER {
				dtMarkCylinderArea(bc.layer, tile.header.Bmin[:], d.m_params.cs, d.m_params.ch,
					ob.cylinder.pos[:], ob.cylinder.radius, ob.cylinder.height, 0)
			} else if ob.Type == DT_OBSTACLE_BOX {
				dtMarkBoxArea(bc.layer, tile.header.Bmin[:], d.m_params.cs, d.m_params.ch,
					ob.box.bmin[:], ob.box.bmax[:], 0)
			} else if ob.Type == DT_OBSTACLE_ORIENTED_BOX {
				dtMarkBoxArea1(bc.layer, tile.header.Bmin[:], d.m_params.cs, d.m_params.ch,
					ob.orientedBox.center[:], ob.orientedBox.halfExtents[:], ob.orientedBox.rotAux[:], 0)
			}
		}
	}

	// Build navmesh
	status = dtBuildTileCacheRegions(bc.layer, walkableClimbVx)
	if status.DtStatusFailed() {
		return status
	}

	bc.lcset = &DtTileCacheContourSet{}
	if bc.lcset == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	status = dtBuildTileCacheContours(bc.layer, walkableClimbVx,
		d.m_params.maxSimplificationError, bc.lcset)
	if status.DtStatusFailed() {
		return status
	}

	bc.lmesh = &DtTileCachePolyMesh{}
	if bc.lmesh == nil {
		return DT_FAILURE | DT_OUT_OF_MEMORY
	}

	status = dtBuildTileCachePolyMesh(bc.lcset, bc.lmesh)
	if status.DtStatusFailed() {
		return status
	}

	// Early out if the mesh tile is empty.
	if bc.lmesh.Npolys == 0 {
		// Remove existing tile.
		navmesh.RemoveTile(navmesh.GetTileRefAt(tile.header.Tx, tile.header.Ty, tile.header.Tlayer))
		return DT_SUCCESS
	}

	var params DtNavMeshCreateParams
	params.Verts = bc.lmesh.Verts
	params.VertCount = bc.lmesh.Nverts
	params.Polys = bc.lmesh.Polys
	params.PolyAreas = bc.lmesh.Areas
	params.PolyFlags = bc.lmesh.Flags
	params.PolyCount = bc.lmesh.Npolys
	params.Nvp = DT_VERTS_PER_POLYGON
	params.WalkableHeight = d.m_params.walkableHeight
	params.WalkableRadius = d.m_params.walkableRadius
	params.WalkableClimb = d.m_params.walkableClimb
	params.TileX = tile.header.Tx
	params.TileY = tile.header.Ty
	params.TileLayer = tile.header.Tlayer
	params.Cs = d.m_params.cs
	params.Ch = d.m_params.ch
	params.BuildBvTree = false
	copy(params.Bmin[:], tile.header.Bmin[:])
	copy(params.Bmax[:], tile.header.Bmax[:])

	if d.m_tmproc != nil {
		d.m_tmproc.process(&params, bc.lmesh.Areas, bc.lmesh.Flags)
	}

	//var navDataSize int
	if !DtCreateNavMeshData(&params) {
		return DT_FAILURE
	}

	// Remove existing tile.
	navmesh.RemoveTile(navmesh.GetTileRefAt(tile.header.Tx, tile.header.Ty, tile.header.Tlayer))

	// Add new tile, or leave the location empty.
	//if navData {
	//	// Let the navmesh own the data.
	//	status = navmesh.addTile(navData, navDataSize, DT_TILE_FREE_DATA, 0, 0)
	//	if status.DtStatusFailed() {
	//		return status
	//	}
	//}

	return DT_SUCCESS
}

func (d *DtTileCache) CalcTightTileBounds(header *DtTileCacheLayerHeader, bmin, bmax []float64) {
	cs := d.m_params.Cs
	bmin[0] = header.Bmin[0] + float64(header.Minx)*cs
	bmin[1] = header.Bmin[1]
	bmin[2] = header.Bmin[2] + float64(header.Miny)*cs
	bmax[0] = header.Bmin[0] + float64(header.Maxx+1)*cs
	bmax[1] = header.Bmax[1]
	bmax[2] = header.Bmin[2] + float64(header.Maxy+1)*cs
}

func (d *DtTileCache) GetObstacleBounds(ob *DtTileCacheObstacle, bmin, bmax []float64) {
	if ob.Type == DT_OBSTACLE_CYLINDER {
		cl := ob.cylinder

		bmin[0] = cl.pos[0] - cl.radius
		bmin[1] = cl.pos[1]
		bmin[2] = cl.pos[2] - cl.radius
		bmax[0] = cl.pos[0] + cl.radius
		bmax[1] = cl.pos[1] + cl.height
		bmax[2] = cl.pos[2] + cl.radius
	} else if ob.Type == DT_OBSTACLE_BOX {
		copy(bmin, ob.box.bmin[:])
		copy(bmax, ob.box.bmax[:])
	} else if ob.Type == DT_OBSTACLE_ORIENTED_BOX {
		orientedBox := ob.orientedBox

		maxr := 1.41 * common.Max(orientedBox.halfExtents[0], orientedBox.halfExtents[2])
		bmin[0] = orientedBox.center[0] - maxr
		bmax[0] = orientedBox.center[0] + maxr
		bmin[1] = orientedBox.center[1] - orientedBox.halfExtents[1]
		bmax[1] = orientedBox.center[1] + orientedBox.halfExtents[1]
		bmin[2] = orientedBox.center[2] - maxr
		bmax[2] = orientedBox.center[2] + maxr
	}
}

func (d *DtTileCache) RemoveObstacle(ref DtObstacleRef) DtStatus {
	if ref == 0 {
		return DT_SUCCESS
	}
	if d.m_nreqs >= MAX_REQUESTS {
		return DT_FAILURE | DT_BUFFER_TOO_SMALL
	}
	req := d.m_reqs[d.m_nreqs]
	if req == nil {
		req = newObstacleRequest()
		d.m_reqs[d.m_nreqs] = req
	}
	d.m_nreqs++
	req.action = REQUEST_REMOVE
	req.ref = ref

	return DT_SUCCESS
}
