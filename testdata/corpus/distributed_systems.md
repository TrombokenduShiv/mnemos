# Distributed Systems and Consensus

Distributed systems are collections of independent computers that appear to users as a single coherent system. They present unique challenges around consistency, availability, fault tolerance, and coordination that do not exist in single-machine systems.

## The CAP Theorem

Brewer's CAP theorem states that a distributed system cannot simultaneously guarantee all three properties: Consistency (every read receives the most recent write), Availability (every request receives a response), and Partition tolerance (the system continues to operate despite network partitions). In practice, since network partitions are unavoidable, systems must choose between consistency and availability during a partition.

## Consensus Algorithms

Consensus algorithms allow distributed systems to agree on a single value or sequence of values despite failures. The Paxos algorithm, developed by Leslie Lamport, was one of the first provably correct consensus protocols. The Raft algorithm, designed as a more understandable alternative to Paxos, has become widely adopted in modern systems like etcd and CockroachDB.

## Replication Strategies

Data replication ensures durability and availability by maintaining copies across multiple nodes. Synchronous replication guarantees consistency but adds latency. Asynchronous replication improves performance but risks data loss if a node fails before replicating. Many systems use a combination, with synchronous replication within a datacenter and asynchronous replication across datacenters.

## Eventual Consistency

Many distributed databases adopt eventual consistency, where replicas may temporarily diverge but will converge to the same state given sufficient time without new updates. This model enables higher availability and lower latency than strong consistency, making it suitable for applications where brief inconsistencies are acceptable.

## Vector Clocks and Conflict Resolution

In eventually consistent systems, concurrent updates can create conflicts. Vector clocks and version vectors track causality between events, helping systems detect conflicts. Conflict resolution strategies include last-writer-wins, merge functions, and operational transformation techniques used in collaborative editing systems.
