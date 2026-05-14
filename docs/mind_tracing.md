# Mind Tracing

1. Server:

   - [x] httpServer
2. Provider

   - [x] basic API for ollama
   - [x] provider status
   - [x] provider controller: status managing

3. Monitor (only for processing)

   - [x] node status
   - [ ] requested provider status (from provider controller)
   - [x] task status

4. Transport

   - [ ] transporter managing inter-node connection
   - [ ] design inter-node payload
5. TaskController 

   - [ ] design task interface or structure
   - [ ] TaskWrapper
   - [ ] TaskDivider
   - [ ] TaskBoard

6. TaskController

   - [ ] TaskAssigner

7. Transport 

   - [ ] trigger from NodeMessage to Action: to taskexecutor

8. TaskExecutor

   - [ ] scheduler
   - [ ] executor

9. Transport

   - [ ] trigger from NodeMessage to Action: to taskcontroller

10. TaskController

    - [ ] TaskReducer