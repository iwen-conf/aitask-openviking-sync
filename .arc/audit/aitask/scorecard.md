# Scorecard — aitask 七维评分（内网单机就绪度）

> **部署假设**（信息源：`docs/API/README.md`）：
> - 不做用户管理、不暴露公网、不上 Kubernetes。
> - docker-compose 单机部署即目标形态。
> - 当前目标：先把业务功能跑通。

评分粒度 0-5（5=优秀，3=可用，2=可发布但需补强，1=有阻断，0=空白）。**Iron Law**：无证据不评分，证据不足直接 `N/A` 并说明缺口。
**总体内网单机就绪度**：**🟢 已可在内网 / 单机投入使用**。补完精简版 P0（启动拒绝默认密钥 + RequestID/访问日志 + 工作树收口）即可进入「跑通业务剧本」阶段。

| # | 维度 | 评分 | 关键证据 | 边界说明 |
|---|---|---|---|---|
| 1 | Architectural Design & Longevity | **3.5 / 5** | E-1, E-7, E-25, E-43 | 多二进制 + 服务依赖清晰、Worker/RPC/HTTP 分层合理；handlers 体量集中（tasks.go 571 行）但尚未失控 |
| 2 | Security Posture & Access Control | **3.5 / 5** ⬆ | E-10..E-14, E-20..E-25 | 鉴权放行是**有意设计**（无用户态 + 内网受信，已写入 `docs/API/README.md`）；剩余风险只有「默认密钥别带进生产环境变量」(E-10..E-12) 与 RequestID 缺失 (E-24)，均属操作风险而非阻断 |
| 3 | Code Quality & Engineering Discipline | **3 / 5** | E-26, E-50, E-51 | 错误码标准化 + 全部单测通过；handlers/worker/context/identity 缺测试；handler 内联 SQL/事务多，可读但难重构 |
| 4 | Business Value & Flow Observability | **3 / 5** | E-43, E-60..E-63, A.4 | 业务剧本与路由/Worker 对齐度高；缺真实运行证据。**这是当前最优先要补的"运行证据"**——见 P1-1 |
| 5 | Observability & Delivery Operations | **3 / 5** ⬆ | E-40..E-45, E-54, E-55 | 健康/就绪/优雅关闭/结构化日志/容器化齐备，docker-compose 单机即目标形态；metrics/trace/k8s/备份/灰度等在内网单机口径下不再列为缺口；唯一仍待补的中间件是 RequestID + 访问日志 |
| 6 | Team Collaboration & Knowledge Flow | **4 / 5** | E-62, E-63, A.6 | 看板 + 决策日志 + 单一信源 + 错误码字典齐备；唯一短板：工作树长期不合并意味着发布单元不清晰 |
| 7 | Technical Debt & Dependency Risk | **N/A**（具体指数） / 文字结论 **2.5 / 5** | A.7, E-3 | Specialist `Dependency Health Score` 需要 SCA + CVE 输入，本次未跑 SBOM；前端版本声明激进、缺 CHANGELOG/tag、in-flight diff 巨大，债务正在累积 |

> **变更说明**（相对原版）：
> - 安全维度：1.5 → 3.5。原扣分依据「无 Auth 自动 operator 放行」「无 CORS」均按公网假设打的分，与本项目「无用户态 + 内网受信」设计意图冲突，已撤销。
> - Ops 维度：2 → 3。原扣分依据「缺 metrics/trace/k8s/备份/回滚」均属公网商用配套，内网单机不需要。

## Specialist Indices

### Business Maturity Index — **N/A**
- `missing_reason`：未取得真实多 Agent 全流程跑通的运行回放、运行日志、关键剧本断点率。
- `missing_evidence_type`：完整业务流的运行回放、关键剧本断点率、故障恢复演练记录。
- `how_to_collect_more_evidence`：补完 P0-B（RequestID + 访问日志）后，跑一次完整业务剧本（创建项目 → 委派给 codex/claude/gemini → 上下文越界触发 handoff → 提交评审 → 自动归档），用 RequestID 串起 trace + room messages + memory artifacts → `.arc/audit/aitask/e2e-baseline/`。

### Dependency Health Score — **N/A**
- `missing_reason`：未运行 SCA（如 `osv-scanner`、`govulncheck`、`pnpm audit`）；前端激进版本号需要交叉核对。
- `missing_evidence_type`：CVE/EOL 报告、版本时效性、维护活跃度、升级自动化（dependabot/renovate）。
- `how_to_collect_more_evidence`：执行 `govulncheck ./...` + `pnpm audit --prod` + `osv-scanner -r .` 并归档输出（recommendations P1-5）。

## Score Boundary（Iron Law 复核）
- 安全 3.5：基于「鉴权放行符合设计意图（E-20 + `docs/API/README.md`）+ 仍有默认密钥操作风险（E-10..E-12）+ RequestID 缺失（E-24）」三条事实综合判定，**不是按公网假设打分**。
- 业务/依赖 specialist 留空，避免给「未知」赋伪精度。
- 整体打分非加权平均；当前部署假设下不存在阻断维度，整体结论给「可在内网 / 单机投入使用」。
