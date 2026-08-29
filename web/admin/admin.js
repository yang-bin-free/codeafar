(() => {
  function run(action) {
    Promise.resolve().then(action).catch(error => feedback(error.message, true));
  }
  function confirmDangerousAction(message, action) {
    run(async () => {
      if (await window.claudePhone.requestConfirmation(message)) await action();
    });
  }
  async function refresh() {
    const token = window.claudePhone.adminToken;
    const response = await fetch("/admin/status", { headers: { Authorization: `Bearer ${token}` } });
    if (!response.ok) throw new Error(`admin status ${response.status}`);
    const { agent, devices, projects, diagnostics, permissionRules, templates } = await response.json();
    document.querySelector("#metrics").innerHTML = [
      ["在线设备", agent.connectedDevices?.length || 0], ["活跃会话", agent.sessions?.length || 0],
      ["Agent", agent.agentVersion], ["Claude", agent.claudeVersion], ["Codex", agent.codexVersion],
      ["运行时间", `${diagnostics.uptimeSeconds}s`], ["内存", `${Math.round(diagnostics.allocBytes / 1048576)} MB`],
      ["Goroutine", diagnostics.goroutines], ["平台", `${diagnostics.goos}/${diagnostics.goarch}`],
      ["防睡眠", diagnostics.caffeinating ? "已启用" : "空闲"]
    ].map(([label, value]) => `<div class="metric-item"><span>${label}</span><strong>${value}</strong></div>`).join("");
    document.querySelector("#settings-workdir").value = agent.defaultWorkingDir || "";
    document.querySelector("#settings-permission").value = agent.defaultPermission || "default";
    document.querySelector("#settings-concurrency").value = agent.maxConcurrentSession || 5;
    document.querySelector("#settings-claude-command").value = agent.claudeCommand || "";
    document.querySelector("#settings-codex-command").value = agent.codexCommand || "";
    renderSessions(agent.sessions || []);
    const deviceList = document.querySelector("#admin-devices");
    deviceList.replaceChildren();
    (devices || []).forEach(device => {
      const row = document.createElement("div");
      row.className = "device-row";
      const label = document.createElement("span");
      label.textContent = `${device.online ? "●" : "○"} ${device.name}`;
      const revoke = document.createElement("button");
      revoke.className = "quiet danger";
      revoke.textContent = "吊销";
      revoke.addEventListener("click", () => confirmDangerousAction("确认吊销这个设备？设备需要重新配对才能连接。", () => revokeDevice(device.deviceId)));
      row.append(label, revoke);
      deviceList.append(row);
    });
    if (!devices?.length) deviceList.textContent = "暂无已授权设备";
    const projectList = document.querySelector("#admin-projects");
    projectList.replaceChildren();
    (projects || []).forEach(project => {
      const row = document.createElement("div");
      row.className = "project-row";
      const label = document.createElement("span");
      label.textContent = `${project.name} — ${project.path}`;
      const remove = document.createElement("button");
      remove.className = "quiet danger";
      remove.textContent = "删除";
      remove.addEventListener("click", () => confirmDangerousAction("确认删除这个工作目录？", () => deleteProject(project.projectId)));
      row.append(label, remove);
      projectList.append(row);
    });
    if (!projects?.length) projectList.textContent = "尚未配置工作目录";
    const templateList = document.querySelector("#admin-templates");
    templateList.replaceChildren();
    (templates || []).forEach(template => {
      const row = document.createElement("div");
      row.className = "template-row";
      const copy = document.createElement("div");
      const label = document.createElement("strong");
      label.textContent = template.label;
      const prompt = document.createElement("p");
      prompt.textContent = template.prompt;
      copy.append(label, prompt);
      const remove = document.createElement("button");
      remove.className = "quiet danger";
      remove.textContent = "删除";
      remove.addEventListener("click", () => confirmDangerousAction("确认删除这个提示词模板？", () => deleteTemplate(template.templateId)));
      row.append(copy, remove);
      templateList.append(row);
    });
    if (!templates?.length) templateList.textContent = "尚未配置提示词模板";
    const permissionList = document.querySelector("#admin-permissions");
    permissionList.replaceChildren();
    (permissionRules || []).forEach(rule => {
      const row = document.createElement("div");
      row.className = "permission-row";
      const label = document.createElement("span");
      label.textContent = rule.pattern ? `${rule.tool}(${rule.pattern})` : rule.tool;
      const remove = document.createElement("button");
      remove.className = "quiet danger";
      remove.textContent = "删除";
      remove.addEventListener("click", () => confirmDangerousAction("确认删除这条权限规则？", () => deletePermissionRule(rule.ruleId)));
      row.append(label, remove);
      permissionList.append(row);
    });
    if (!permissionRules?.length) permissionList.textContent = "尚未记忆权限规则";
  }
  function feedback(message, isError = false) {
    const node = document.querySelector("#admin-feedback");
    node.textContent = message;
    node.classList.toggle("error", isError);
  }
  function renderSessions(sessions) {
    const list = document.querySelector("#admin-sessions");
    list.replaceChildren();
    sessions.forEach(session => {
      const row = document.createElement("div");
      row.className = "session-admin-row";
      const copy = document.createElement("div");
      const name = document.createElement("strong");
      name.textContent = session.name || session.sessionId;
      const meta = document.createElement("p");
      meta.textContent = `${session.health || "idle"} · ${session.running ? "运行中" : "空闲"} · ${session.subscribers?.length || 0} 个订阅者`;
      copy.append(name, meta);
      const stop = document.createElement("button");
      stop.className = "quiet danger";
      stop.textContent = "停止";
      stop.addEventListener("click", () => confirmDangerousAction("确认停止这个会话？未完成的输出会中断。", () => stopSession(session.sessionId)));
      row.append(copy, stop);
      list.append(row);
    });
    if (!sessions.length) list.textContent = "暂无活跃会话";
  }
  async function revokeDevice(deviceId) {
    const response = await fetch(`/admin/devices/${encodeURIComponent(deviceId)}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${window.claudePhone.adminToken}` }
    });
    if (!response.ok) throw new Error(`revoke device ${response.status}`);
    await refresh();
  }
  async function deleteProject(projectId) {
    const response = await fetch(`/admin/projects/${encodeURIComponent(projectId)}`, {
      method: "DELETE", headers: { Authorization: `Bearer ${window.claudePhone.adminToken}` }
    });
    if (!response.ok) throw new Error(`delete project ${response.status}`);
    await refresh();
  }
  async function deletePermissionRule(ruleId) {
    const response = await fetch(`/admin/permission-rules/${encodeURIComponent(ruleId)}`, {
      method: "DELETE", headers: { Authorization: `Bearer ${window.claudePhone.adminToken}` }
    });
    if (!response.ok) throw new Error(`delete permission rule ${response.status}`);
    await refresh();
  }
  async function deleteTemplate(templateId) {
    const response = await fetch(`/admin/templates/${encodeURIComponent(templateId)}`, {
      method: "DELETE", headers: { Authorization: `Bearer ${window.claudePhone.adminToken}` }
    });
    if (!response.ok) throw new Error(`delete template ${response.status}`);
    feedback("模板已删除");
    await refresh();
  }
  async function stopSession(sessionId) {
    const response = await fetch("/admin/sessions/stop", {
      method: "POST",
      headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId })
    });
    if (!response.ok) throw new Error(await response.text());
    feedback("会话已停止");
    await refresh();
  }
  document.querySelector("#settings-form").addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const response = await fetch("/admin/settings", {
        method: "PATCH",
        headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
        body: JSON.stringify({
          defaultWorkingDir: document.querySelector("#settings-workdir").value.trim(),
          defaultPermission: document.querySelector("#settings-permission").value,
          maxConcurrentSessions: Number(document.querySelector("#settings-concurrency").value),
          claudeCommand: document.querySelector("#settings-claude-command").value.trim(),
          codexCommand: document.querySelector("#settings-codex-command").value.trim()
        })
      });
      if (!response.ok) throw new Error(await response.text());
      feedback("运行设置已保存");
      await refresh();
    } catch (error) { feedback(error.message, true); }
  });
  document.querySelector("#template-form").addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const response = await fetch("/admin/templates", {
        method: "POST",
        headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
        body: JSON.stringify({ label: document.querySelector("#template-label").value.trim(), prompt: document.querySelector("#template-prompt").value.trim() })
      });
      if (!response.ok) throw new Error(await response.text());
      event.target.reset();
      feedback("模板已添加");
      await refresh();
    } catch (error) { feedback(error.message, true); }
  });
  document.querySelector("#project-form").addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const response = await fetch("/admin/projects", {
        method: "POST",
        headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
        body: JSON.stringify({ name: document.querySelector("#project-name").value.trim(), path: document.querySelector("#project-path").value.trim(), permission: "default" })
      });
      if (!response.ok) throw new Error(await response.text());
      event.target.reset();
      feedback("工作目录已添加");
      await refresh();
    } catch (error) { feedback(error.message, true); }
  });
  document.querySelector("#device-form").addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const response = await fetch("/admin/devices", {
        method: "POST",
        headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
        body: JSON.stringify({ name: document.querySelector("#device-name").value.trim() || "Android" })
      });
      if (!response.ok) throw new Error(await response.text());
      const credential = await response.json();
      const output = document.querySelector("#new-device-token");
      output.hidden = false;
      output.textContent = `请复制到手机（只显示一次）：\n${credential.deviceToken}`;
      event.target.reset();
      await refresh();
    } catch (error) { feedback(error.message, true); }
  });
  document.querySelector("#permission-form").addEventListener("submit", async event => {
    event.preventDefault();
    try {
      const response = await fetch("/admin/permission-rules", {
        method: "POST",
        headers: { Authorization: `Bearer ${window.claudePhone.adminToken}`, "Content-Type": "application/json" },
        body: JSON.stringify({ tool: document.querySelector("#permission-tool").value.trim(), pattern: document.querySelector("#permission-pattern").value.trim() })
      });
      if (!response.ok) throw new Error(await response.text());
      event.target.reset();
      feedback("权限规则已添加");
      await refresh();
    } catch (error) { feedback(error.message, true); }
  });
  window.claudePhone.refreshAdmin = refresh;
})();
