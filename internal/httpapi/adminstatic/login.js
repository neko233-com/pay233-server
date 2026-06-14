async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

const loginForm = document.querySelector("#loginForm");
const loginError = document.querySelector("#loginError");

loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  loginError.textContent = "";
  const form = new FormData(loginForm);
  try {
    await api("/admin/login", {
      method: "POST",
      body: JSON.stringify({
        username: form.get("username"),
        password: form.get("password"),
      }),
    });
    window.location.assign("/admin/dashboard.html");
  } catch {
    loginError.textContent = "账号或密码错误";
  }
});

(async function boot() {
  try {
    const session = await api("/admin/api/session");
    if (session.authenticated) {
      window.location.replace("/admin/dashboard.html");
    }
  } catch {
  }
})();
