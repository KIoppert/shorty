const $ = (sel) => document.querySelector(sel);

const state = { user: null, links: [], current: null, authMode: "login" };

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && state.user) showAuth(data.error);
  if (!res.ok) throw new Error(data.error || "Сервер не ответил, попробуйте ещё раз");
  return data;
}

const plural = (n, one, few, many) => {
  const rule = new Intl.PluralRules("ru").select(n);
  return rule === "one" ? one : rule === "few" ? few : many;
};

const dateFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "short" });
const timeFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

const isExpired = (link) => link.expires_at && new Date(link.expires_at) < new Date();

const toLocalInput = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
};

const fromLocalInput = (value) => (value ? new Date(value).toISOString() : null);

function toast(text) {
  const el = $("#toast");
  el.textContent = text;
  el.classList.add("toast--show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => el.classList.remove("toast--show"), 1800);
}

async function copy(text) {
  await navigator.clipboard.writeText(text);
  toast("Ссылка скопирована");
}

function showAuth(message) {
  state.user = null;
  $("#auth-form .error").textContent = typeof message === "string" ? message : "";
  $("#app").hidden = true;
  $("#auth").hidden = false;
  $("#details").close();
}

async function showApp(user) {
  state.user = user;
  $("#user-email").textContent = user.email;
  $("#nav").hidden = !user.is_admin;
  $("#auth").hidden = true;
  $("#app").hidden = false;
  await route();
}

async function route() {
  if (!state.user) return;
  const view = location.hash === "#admin" && state.user.is_admin ? "admin" : "links";
  $("#view-links").hidden = view !== "links";
  $("#view-admin").hidden = view !== "admin";
  document.querySelectorAll("[data-view]").forEach((a) => a.toggleAttribute("aria-current", a.dataset.view === view));

  if (view === "admin") {
    await loadAdmin();
  } else {
    state.links = await api("GET", "/api/links");
    renderLinks();
  }
}

function setAuthMode(mode) {
  state.authMode = mode;
  const form = $("#auth-form");
  form.querySelectorAll("[role=tab]").forEach((tab) => tab.setAttribute("aria-selected", tab.dataset.mode === mode));
  form.querySelector("[type=submit]").textContent = mode === "login" ? "Войти" : "Создать аккаунт";
  form.password.autocomplete = mode === "login" ? "current-password" : "new-password";
  form.querySelector(".error").textContent = "";
}

function renderLinks(newId) {
  const list = $("#links");
  list.replaceChildren(...state.links.map((link) => renderTicket(link, link.id === newId)));
  $("#links-count").textContent = state.links.length ? `· ${state.links.length}` : "";
  $("#links-empty").hidden = state.links.length > 0;
}

function renderTicket(link, isNew) {
  const node = $("#ticket-tpl").content.firstElementChild.cloneNode(true);
  const off = !link.enabled || isExpired(link);
  node.classList.toggle("ticket--off", off);
  node.classList.toggle("ticket--new", isNew);

  node.querySelector(".stub__code").textContent = `/${link.code}`;
  node.querySelector(".ticket__title").textContent = link.title || new URL(link.url).hostname;
  node.querySelector(".ticket__url").textContent = link.url;

  const meta = node.querySelector(".ticket__meta");
  const clicks = document.createElement("span");
  clicks.innerHTML = `<b>${link.clicks}</b> ${plural(link.clicks, "переход", "перехода", "переходов")}`;
  meta.append(clicks);
  if (link.expires_at) meta.append(tag(isExpired(link) ? "истекла" : `до ${dateFmt.format(new Date(link.expires_at))}`, isExpired(link)));
  if (!link.enabled) meta.append(tag("выключена", true));

  node.querySelector("[data-action=copy]").addEventListener("click", () => copy(link.short_url));
  node.querySelector("[data-action=open]").addEventListener("click", () => openDetails(link));
  return node;
}

function tag(text, off) {
  const el = document.createElement("span");
  el.className = off ? "tag tag--off" : "tag";
  el.textContent = text;
  return el;
}

async function openDetails(link) {
  state.current = link;
  const short = $("#d-short");
  short.href = link.short_url;
  short.textContent = link.short_url.replace(/^https?:\/\//, "");
  $("#d-qr").src = `/api/links/${link.id}/qr`;
  $("#d-qr-download").href = `/api/links/${link.id}/qr`;
  $("#d-qr-download").download = `shorty-${link.code}.png`;

  const form = $("#edit-form");
  form.url.value = link.url;
  form.title.value = link.title;
  form.expires_at.value = toLocalInput(link.expires_at);
  form.enabled.checked = link.enabled;
  form.querySelector(".error").textContent = "";

  $("#d-total").textContent = link.clicks;
  $("#d-total-label").textContent = plural(link.clicks, "переход", "перехода", "переходов");
  $("#d-chart").replaceChildren();
  $("#d-recent").replaceChildren();
  $("#details").showModal();

  const stats = await api("GET", `/api/links/${link.id}/stats`);
  renderStats(stats);
}

function renderStats(stats) {
  const max = Math.max(1, ...stats.daily.map((d) => d.count));
  $("#d-chart").replaceChildren(...stats.daily.map((d) => {
    const bar = document.createElement("span");
    bar.style.height = `${(d.count / max) * 100}%`;
    bar.title = `${dateFmt.format(new Date(d.day))}: ${d.count}`;
    if (!d.count) bar.dataset.zero = "";
    return bar;
  }));

  const devices = { desktop: "компьютер", mobile: "телефон", tablet: "планшет", bot: "бот" };
  $("#d-recent").replaceChildren(...stats.recent.map((c) => {
    const li = document.createElement("li");
    const source = document.createElement("b");
    source.textContent = c.referrer || "прямой переход";
    const info = document.createElement("span");
    info.textContent = `${devices[c.device] || c.device} · ${timeFmt.format(new Date(c.clicked_at))}`;
    li.append(source, info);
    return li;
  }));
  if (!stats.recent.length) $("#d-recent").innerHTML = "<li>Переходов пока не было</li>";
}

function replaceLink(updated) {
  state.links = state.links.map((l) => (l.id === updated.id ? updated : l));
  renderLinks();
}

$("#auth-form").addEventListener("click", (e) => {
  if (e.target.dataset.mode) setAuthMode(e.target.dataset.mode);
});

$("#auth-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target;
  try {
    const user = await api("POST", `/api/auth/${state.authMode}`, {
      email: form.email.value,
      password: form.password.value,
    });
    form.reset();
    await showApp(user);
  } catch (err) {
    form.querySelector(".error").textContent = err.message;
  }
});

$("#logout").addEventListener("click", async () => {
  await api("POST", "/api/auth/logout");
  showAuth();
});

$("#delete-account").addEventListener("click", async () => {
  if (!confirm("Удалить аккаунт вместе со всеми ссылками? Это нельзя отменить.")) return;
  await api("DELETE", "/api/me");
  showAuth();
});

$("#create-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target;
  const error = form.querySelector(".error");
  try {
    const link = await api("POST", "/api/links", {
      url: form.url.value,
      alias: form.alias.value,
      title: form.title.value,
      expires_at: fromLocalInput(form.expires_at.value),
    });
    form.reset();
    error.textContent = "";
    state.links.unshift(link);
    renderLinks(link.id);
    copy(link.short_url).catch(() => toast("Ссылка создана"));
  } catch (err) {
    error.textContent = err.message;
  }
});

$("#edit-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target;
  try {
    const link = await api("PUT", `/api/links/${state.current.id}`, {
      url: form.url.value,
      title: form.title.value,
      enabled: form.enabled.checked,
      expires_at: fromLocalInput(form.expires_at.value),
    });
    replaceLink(link);
    $("#details").close();
    toast("Изменения сохранены");
  } catch (err) {
    form.querySelector(".error").textContent = err.message;
  }
});

$("#d-copy").addEventListener("click", () => copy(state.current.short_url));

$("#d-delete").addEventListener("click", async () => {
  if (!confirm("Удалить ссылку и её статистику?")) return;
  await api("DELETE", `/api/links/${state.current.id}`);
  state.links = state.links.filter((l) => l.id !== state.current.id);
  renderLinks();
  $("#details").close();
  toast("Ссылка удалена");
});

window.addEventListener("hashchange", () => route().catch((err) => toast(err.message)));

api("GET", "/api/me").then(showApp).catch(showAuth);
