const admin = { tab: "users", query: "", timer: null };

function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children.filter((c) => c !== null && c !== false));
  return node;
}

const badge = (text, kind = "") => el("span", { className: `badge ${kind}`, textContent: text });

const button = (text, onClick, kind = "") =>
  el("button", { type: "button", className: `btn btn--small ${kind}`, textContent: text, onclick: onClick });

async function loadAdmin() {
  const search = encodeURIComponent(admin.query);
  const [overview, items] = await Promise.all([
    api("GET", "/api/admin/overview"),
    api("GET", `/api/admin/${admin.tab}?q=${search}`),
  ]);
  renderOverview(overview);
  const rows = items.map(admin.tab === "users" ? userRow : linkRow);
  $("#admin-list").replaceChildren(...rows);
  $("#admin-empty").hidden = rows.length > 0;
}

function renderOverview(o) {
  const stat = (label, value, note) =>
    el("div", { className: "overview__item" },
      el("dt", { textContent: label }),
      el("dd", { textContent: value }),
      note ? el("small", { textContent: note }) : null);

  $("#overview").replaceChildren(
    stat("Пользователи", o.users, o.banned ? `${o.banned} в бане` : null),
    stat("Ссылки", o.links),
    stat("Переходы", o.clicks),
    stat("Сегодня", o.clicks_today),
  );
}

function userRow(u) {
  const isMe = u.id === state.user.id;
  const meta = [
    `${u.links} ${plural(u.links, "ссылка", "ссылки", "ссылок")}`,
    `${u.clicks} ${plural(u.clicks, "переход", "перехода", "переходов")}`,
    `с ${dateFmt.format(new Date(u.created_at))}`,
  ].join(" · ");

  return el("li", { className: u.banned_at ? "row row--off" : "row" },
    el("div", { className: "row__main" },
      el("div", { className: "row__title" },
        el("span", { textContent: u.email }),
        u.is_admin && badge("админ", "badge--accent"),
        u.banned_at && badge("заблокирован", "badge--danger"),
        isMe && badge("это вы")),
      el("span", { className: "row__meta", textContent: meta })),
    isMe ? null : el("div", { className: "row__actions" },
      button(u.is_admin ? "Снять админа" : "Сделать админом", () => updateUser(u, { is_admin: !u.is_admin })),
      button(u.banned_at ? "Разблокировать" : "Заблокировать", () => updateUser(u, { banned: !u.banned_at }), u.banned_at ? "" : "btn--warn"),
      button("Удалить", () => deleteUser(u), "btn--danger")));
}

function linkRow(l) {
  const toggle = el("input", { type: "checkbox", checked: l.enabled, title: "Ссылка включена" });
  toggle.addEventListener("change", () => setLinkEnabled(l, toggle.checked));

  const meta = [l.owner_email, `${l.clicks} ${plural(l.clicks, "переход", "перехода", "переходов")}`];
  if (isExpired(l)) meta.push("истекла");
  if (l.owner_banned) meta.push("владелец заблокирован");
  const works = l.enabled && !isExpired(l) && !l.owner_banned;

  return el("li", { className: works ? "row" : "row row--off" },
    el("div", { className: "row__main" },
      el("div", { className: "row__title" },
        el("a", { className: "chip", href: l.short_url, target: "_blank", rel: "noopener", textContent: `/${l.code}` }),
        el("span", { className: "row__url", textContent: l.url })),
      el("span", { className: "row__meta", textContent: meta.join(" · ") })),
    el("div", { className: "row__actions" },
      el("label", { className: "switch" }, toggle),
      button("Удалить", () => deleteAdminLink(l), "btn--danger")));
}

async function adminAction(message, fn) {
  if (message && !confirm(message)) return;
  try {
    await fn();
    await loadAdmin();
  } catch (err) {
    toast(err.message);
  }
}

const updateUser = (u, change) =>
  adminAction(change.banned ? `Заблокировать ${u.email}? Все ссылки пользователя перестанут работать.` : null,
    () => api("PATCH", `/api/admin/users/${u.id}`, change));

const deleteUser = (u) =>
  adminAction(`Удалить ${u.email} вместе со всеми ссылками?`, () => api("DELETE", `/api/admin/users/${u.id}`));

const setLinkEnabled = (l, enabled) =>
  adminAction(null, () => api("PATCH", `/api/admin/links/${l.id}`, { enabled }));

const deleteAdminLink = (l) =>
  adminAction(`Удалить ссылку /${l.code}?`, () => api("DELETE", `/api/admin/links/${l.id}`));

$("#view-admin .tabs").addEventListener("click", (e) => {
  const tab = e.target.dataset.tab;
  if (!tab) return;
  admin.tab = tab;
  document.querySelectorAll("[data-tab]").forEach((t) => t.setAttribute("aria-selected", t.dataset.tab === tab));
  loadAdmin().catch((err) => toast(err.message));
});

$("#admin-search").addEventListener("input", (e) => {
  clearTimeout(admin.timer);
  admin.timer = setTimeout(() => {
    admin.query = e.target.value.trim();
    loadAdmin().catch((err) => toast(err.message));
  }, 250);
});
