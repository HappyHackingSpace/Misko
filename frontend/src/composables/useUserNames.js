// Who did something, by user id. Only user managers may list users, so other
// roles see "you" for themselves and "another user" for everyone else.
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api/client.js";
import { useAuth } from "../stores/auth.js";

const names = ref({});
let loading = null;

export function useUserNames() {
  const { t } = useI18n();
  const auth = useAuth();

  if (!loading && auth.can("user:manage")) {
    loading = api("/users", { query: { pageSize: 100 } })
      .then((page) => (names.value = Object.fromEntries((page.data || []).map((u) => [u.id, u.name || u.email]))))
      .catch(() => (loading = null));
  }

  function userName(id) {
    if (!id) return "";
    if (id === auth.user?.id) return t("people.you");
    return names.value[id] || t("people.someone");
  }
  return { userName };
}
