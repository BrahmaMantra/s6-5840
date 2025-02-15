## Part A: The Controller and Static Sharding (easy)
实现必须支持 shardctrler/common.go 中描述的 RPC 接口，该接口由 Join、Leave、Move 和 Query RPC 组成。这些 RPC 旨在允许管理员（和测试）控制 shardctrler：
- 添加新的副本组、消除副本组以及在副本组之间移动分片。

### Join RPC
管理员使用 Join RPC 来**添加新的副本组(replica groups)**。其参数是一组从唯一的非零副本组标识符 (**GID**) 到**服务器名称列表**的映射。shardctrler 应该通过创建包含新副本组的新配置来做出反应。新配置应尽可能均匀地将分片分配到所有组中，并应尽可能少地移动分片以实现该目标。如果 GID 不是当前配置的一部分，则 shardctrler 应允许重复使用 GID（即应允许 GID 加入，然后离开，然后再次加入）。

### Leave RPC 
Leave RPC的参数是先前加入的组的 GID 列表。shardctrler 应创建一个不包含这些组的新配置，并将这些组的分片分配给剩余组。新配置应尽可能均匀地将分片分配到所有组中，并应尽可能少地移动分片以实现该目标。

### Move RPC
Move RPC 的参数是分片编号(shard number)和 GID。shardctrler 应创建一个新配置，其中将分片分配给组。Move 的目的是让我们测试您的软件。A Join or Leave following a Move可能会取消移动，因为加入和离开会重新平衡。

### Query RPC
Query  RPC 的参数是配置编号。shardctrler 使用具有该编号的配置进行回复。如果该编号为 -1 或大于已知最大配置编号，shardctrler 应使用最新配置进行回复。查询 (-1) 的结果应反映 shardctrler 在收到查询 (-1) RPC 之前完成处理的每个加入、离开或移动 RPC。

- 您必须在 shardctrler/ 目录中的 client.go 和 server.go 中实现上述指定的接口。您的 shardctrler 必须具有容错能力，并使用lab3/4中的 Raft 库。当您通过 shardctrler/ 中的所有测试时，您就完成了此任务。