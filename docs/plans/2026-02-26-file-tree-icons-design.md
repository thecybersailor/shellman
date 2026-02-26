# File Tree Icons Design (2026-02-26)

## 目标
将右侧 file tree 的“文件图标”替换为 `dmhendricks/file-icon-vectors` 风格；文件夹图标保持现状。

## 范围
- 修改前端 `webui`。
- 仅影响 `FilePanel` 中文件节点图标渲染。
- 不新增后端接口、不改协议、不加配置。

## 关键约束
- 工作模式：`worktree + TDD`。
- 依赖管理：使用 `npm`。
- 图标查找尽量 `O(1)`：通过后缀名哈希映射 `Record<string, Component>` 直接命中。

## 方案
1. 通过 npm 安装图标依赖（基于 file-icon-vectors 资源）。
2. 在 `FilePanel.vue` 新增扩展名到图标组件的常量映射表。
3. 新增 `resolveFileIcon(name: string)`：
   - 解析文件名后缀并规范化为小写。
   - 通过映射表 `O(1)` 查找。
   - 未命中返回默认文件图标。
4. 渲染时对文件节点使用解析结果，目录节点继续使用 `Folder/FolderOpen`。

## 测试策略（TDD）
- RED：先在 `FilePanel.spec.ts` 添加失败测试，验证：
  - `.ts` 文件使用映射图标而非默认文件图标。
  - 未知后缀使用默认文件图标。
- GREEN：实现最小代码使测试通过。
- REFACTOR：整理映射与函数命名，保持单一路径。

## 风险
- 第三方图标包导出结构可能与预期不一致，需要以实际包结构调整导入方式。
