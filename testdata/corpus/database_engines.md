# Database Storage Engines

Storage engines are the core component of any database system, responsible for how data is stored, retrieved, and maintained on disk and in memory. The choice of storage engine fundamentally affects a database's performance characteristics.

## B-Tree Based Engines

B-trees and their variants (B+ trees) are the most widely used data structure for database storage engines. They maintain sorted data and allow searches, sequential access, insertions, and deletions in logarithmic time. Most traditional relational databases like PostgreSQL, MySQL (InnoDB), and Oracle use B-tree based storage.

B-trees are optimized for read-heavy workloads because they minimize the number of disk seeks needed to find a record. However, random writes can be expensive because each write may require updating multiple tree nodes across different disk locations.

## Log-Structured Merge Trees (LSM Trees)

LSM trees take a fundamentally different approach: all writes are sequential. Data is first written to an in-memory structure (memtable), then periodically flushed to immutable, sorted files on disk called Sorted String Tables (SSTables). Background compaction merges these files to maintain read performance.

LSM trees power databases designed for write-heavy workloads, including LevelDB, RocksDB, Cassandra, and HBase. Their sequential write pattern is particularly well-suited for SSDs and modern storage hardware.

## Write-Ahead Logging

Most storage engines use a Write-Ahead Log (WAL) to ensure durability. Before any modification is applied to the main data structure, a record of the change is appended to a sequential log file and fsynced to disk. If the system crashes, the WAL can be replayed to recover any committed transactions that hadn't been applied yet.

The WAL is critical for crash recovery: it guarantees that no acknowledged write is lost, even if the process is killed mid-operation. The log uses checksums (typically CRC32) to detect and discard any partially-written records from a crash.

## Concurrency Control

Modern storage engines must handle concurrent reads and writes efficiently. Common approaches include multi-version concurrency control (MVCC), which allows readers to see a consistent snapshot without blocking writers, and various locking strategies from coarse-grained table locks to fine-grained row or key-range locks.

## Compaction Strategies

LSM tree engines must periodically compact their on-disk files to bound read amplification (the number of files that must be consulted for a single read). Two main strategies exist: size-tiered compaction (merging similarly-sized files) and leveled compaction (maintaining strict size ratios between levels). The choice affects write amplification, read amplification, and space amplification trade-offs.
