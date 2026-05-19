# Log

> 2026.05.13

1. 完成 `Monitor` 的接口工作
2. 调整 `TaskRuntimeStatus` 的获取时机，从 `Provider` 中获取转为 `Monitor` 全权负责
3. `ProviderController` 调整为仅负责接收 `Task` 状态，不再负责 `Task` 相关管理

> 2026.05.14

1. 在 `api` 中加入 `event.go` 负责消息队列，专门供给 `Monitor` 异步获取 `Status` （结构未设计完备）

> 2026.05.18

1. add `Publish` on `Transporter`
2. maintain `libp2p/SubscribeProtocol` stream to only one
3. add `NodePostman` interface under `transport`

> 2026.05.19

1. adding `NodePayload` under `api`
   1. add `TaskBroadcastMsg` under `api`
   2. add `TaskRequestMsg` 
   3. add `TaskAssignMsg`
   4. add `TaskAcceptMsg`
   5. add `TaskResultMsg`
2. adding `TaskImage` under `api`

3. start `routecontroller`
