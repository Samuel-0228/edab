package streaming

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DurableStreamConfig sets options for file-backed streaming persistence.
type DurableStreamConfig struct {
	DataDir       string
	Partitions    int
	SyncOnWrite   bool
	RetentionDays int
}

// DurableStreamAdapter implements a production-grade, append-only WAL stream broker.
// It persists records to partitioned segment files on disk, ensuring durability across server restarts.
type DurableStreamAdapter struct {
	mu           sync.RWMutex
	dataDir      string
	partitions   int
	syncOnWrite  bool
	offsetStore  OffsetStore
	partitioner  PartitionStrategy
	writers      map[string]map[int32]*partitionWriter
	groupOffsets map[string]map[string]map[int32]int64
	closed       bool
	ctx          context.Context
	cancel       context.CancelFunc
}

type partitionWriter struct {
	file   *os.File
	writer *bufio.Writer
	offset int64
}

// NewDurableStreamAdapter creates or opens an on-disk partitioned streaming log.
func NewDurableStreamAdapter(cfg DurableStreamConfig, offsetStore OffsetStore) (*DurableStreamAdapter, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(os.TempDir(), "kerob_streams")
	}
	if cfg.Partitions <= 0 {
		cfg.Partitions = 4
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create stream data directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	adapter := &DurableStreamAdapter{
		dataDir:      cfg.DataDir,
		partitions:   cfg.Partitions,
		syncOnWrite:  cfg.SyncOnWrite,
		offsetStore:  offsetStore,
		partitioner:  DefaultPartitionStrategy,
		writers:      make(map[string]map[int32]*partitionWriter),
		groupOffsets: make(map[string]map[string]map[int32]int64),
		ctx:          ctx,
		cancel:       cancel,
	}

	return adapter, nil
}

func (d *DurableStreamAdapter) Name() string {
	return "durable-wal"
}

func (d *DurableStreamAdapter) Producer() (Producer, error) {
	return d, nil
}

func (d *DurableStreamAdapter) Consumer() (Consumer, error) {
	return d, nil
}

func (d *DurableStreamAdapter) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true
	d.cancel()

	for _, partMap := range d.writers {
		for _, pw := range partMap {
			if pw.writer != nil {
				_ = pw.writer.Flush()
			}
			if pw.file != nil {
				_ = pw.file.Close()
			}
		}
	}
	return nil
}

func (d *DurableStreamAdapter) Publish(ctx context.Context, record *Record) (*PublishResult, error) {
	res, err := d.PublishBatch(ctx, []*Record{record})
	if err != nil {
		return nil, err
	}
	return res[0], nil
}

func (d *DurableStreamAdapter) PublishBatch(ctx context.Context, records []*Record) ([]*PublishResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, ErrClosed
	}

	results := make([]*PublishResult, 0, len(records))
	now := time.Now().UTC()

	for _, rec := range records {
		if rec.Topic == "" {
			rec.Topic = "events"
		}
		if rec.ID == "" {
			rec.ID = uuid.NewString()
		}
		if rec.Timestamp.IsZero() {
			rec.Timestamp = now
		}

		if rec.Partition < 0 || rec.Partition >= int32(d.partitions) {
			rec.Partition = d.partitioner.AssignPartition(rec.OrderingKey, d.partitions)
		}

		pw, err := d.getOrCreateWriter(rec.Topic, rec.Partition)
		if err != nil {
			return nil, fmt.Errorf("failed to open partition writer: %w", err)
		}

		rec.Offset = pw.offset

		line, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize stream record: %w", err)
		}
		line = append(line, '\n')

		if _, err := pw.writer.Write(line); err != nil {
			return nil, fmt.Errorf("failed to write to partition log: %w", err)
		}
		if d.syncOnWrite {
			_ = pw.writer.Flush()
			_ = pw.file.Sync()
		}

		pw.offset++

		results = append(results, &PublishResult{
			ID:        rec.ID,
			Topic:     rec.Topic,
			Partition: rec.Partition,
			Offset:    rec.Offset,
			Timestamp: rec.Timestamp,
		})
	}

	// Flush all touched writers
	for _, partMap := range d.writers {
		for _, pw := range partMap {
			_ = pw.writer.Flush()
		}
	}

	return results, nil
}

func (d *DurableStreamAdapter) getOrCreateWriter(topic string, partition int32) (*partitionWriter, error) {
	if _, ok := d.writers[topic]; !ok {
		d.writers[topic] = make(map[int32]*partitionWriter)
	}
	if pw, ok := d.writers[topic][partition]; ok {
		return pw, nil
	}

	topicDir := filepath.Join(d.dataDir, topic)
	if err := os.MkdirAll(topicDir, 0755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(topicDir, fmt.Sprintf("partition-%03d.log", partition))
	offset := countExistingRecords(logPath)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	pw := &partitionWriter{
		file:   f,
		writer: bufio.NewWriter(f),
		offset: offset,
	}
	d.writers[topic][partition] = pw
	return pw, nil
}

func countExistingRecords(logPath string) int64 {
	f, err := os.Open(logPath)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	var count int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func (d *DurableStreamAdapter) Subscribe(ctx context.Context, groupID string, topics []string, handler MessageHandler) error {
	for _, topic := range topics {
		for p := int32(0); p < int32(d.partitions); p++ {
			part := p
			top := topic
			go d.pollPartitionFile(ctx, groupID, top, part, handler)
		}
	}
	return nil
}

func (d *DurableStreamAdapter) pollPartitionFile(ctx context.Context, groupID, topic string, partition int32, handler MessageHandler) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			currentOffset, _ := d.GetCommittedOffset(ctx, groupID, topic, partition)
			records, err := d.readRecordsFromDisk(topic, partition, currentOffset, 50)
			if err != nil || len(records) == 0 {
				continue
			}

			for _, rec := range records {
				if err := handler(ctx, rec); err == nil {
					_ = d.CommitOffset(ctx, groupID, topic, partition, rec.Offset+1)
				}
			}
		}
	}
}

func (d *DurableStreamAdapter) readRecordsFromDisk(topic string, partition int32, fromOffset int64, limit int) ([]*Record, error) {
	logPath := filepath.Join(d.dataDir, topic, fmt.Sprintf("partition-%03d.log", partition))
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var result []*Record
	scanner := bufio.NewScanner(f)
	var currentOffset int64

	for scanner.Scan() {
		if currentOffset >= fromOffset {
			var rec Record
			if err := json.Unmarshal(scanner.Bytes(), &rec); err == nil {
				result = append(result, &rec)
				if limit > 0 && len(result) >= limit {
					break
				}
			}
		}
		currentOffset++
	}

	return result, scanner.Err()
}

func (d *DurableStreamAdapter) CommitOffset(ctx context.Context, groupID, topic string, partition int32, offset int64) error {
	d.mu.Lock()
	if _, ok := d.groupOffsets[groupID]; !ok {
		d.groupOffsets[groupID] = make(map[string]map[int32]int64)
	}
	if _, ok := d.groupOffsets[groupID][topic]; !ok {
		d.groupOffsets[groupID][topic] = make(map[int32]int64)
	}
	d.groupOffsets[groupID][topic][partition] = offset
	d.mu.Unlock()

	if d.offsetStore != nil {
		return d.offsetStore.CommitOffset(ctx, ConsumerOffset{
			GroupID:     groupID,
			Topic:       topic,
			Partition:   partition,
			Offset:      offset,
			CommittedAt: time.Now().UTC(),
		})
	}
	return nil
}

func (d *DurableStreamAdapter) GetCommittedOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error) {
	if d.offsetStore != nil {
		off, err := d.offsetStore.GetOffset(ctx, groupID, topic, partition)
		if err == nil {
			return off, nil
		}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if gMap, ok := d.groupOffsets[groupID]; ok {
		if tMap, ok := gMap[topic]; ok {
			return tMap[partition], nil
		}
	}
	return 0, nil
}

func (d *DurableStreamAdapter) Seek(ctx context.Context, topic string, partition int32, offset int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, tMap := range d.groupOffsets {
		if pMap, ok := tMap[topic]; ok {
			pMap[partition] = offset
		}
	}
	return nil
}

// -------------------------------------------------------------------------
// Pluggable Driver Registry (Kafka / NATS / Cloud Queues)
// -------------------------------------------------------------------------

// DriverFactory instantiates a custom broker driver from configuration.
type DriverFactory func(config map[string]any) (StreamAdapter, error)

var (
	driverMu      sync.RWMutex
	streamDrivers = make(map[string]DriverFactory)
)

// RegisterDriver registers an external broker driver (e.g., "kafka", "nats", "sqs").
func RegisterDriver(name string, factory DriverFactory) {
	driverMu.Lock()
	defer driverMu.Unlock()
	streamDrivers[name] = factory
}

// CreateAdapter instantiates a stream adapter by name.
func CreateAdapter(name string, config map[string]any) (StreamAdapter, error) {
	driverMu.RLock()
	factory, ok := streamDrivers[name]
	driverMu.RUnlock()

	if ok {
		return factory(config)
	}

	switch name {
	case "memory", "local":
		return NewMemoryStreamAdapter(4), nil
	case "durable", "wal", "file":
		dataDir := ""
		if d, ok := config["data_dir"].(string); ok {
			dataDir = d
		}
		return NewDurableStreamAdapter(DurableStreamConfig{
			DataDir:     dataDir,
			Partitions:  4,
			SyncOnWrite: true,
		}, nil)
	default:
		return nil, fmt.Errorf("unknown streaming adapter driver: %q", name)
	}
}
