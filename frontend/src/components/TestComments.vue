<script setup>
import { ref, computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useAuth } from "../stores/auth.js";

// Discussion thread for a single test. Any authenticated user can post; the
// author (or a privileged role) can delete. Bodies are rendered with text
// interpolation only (never v-html), so stored content is always escaped by Vue
// and cannot execute - the XSS defense lives here, at the render boundary.
const props = defineProps({ testId: { type: String, required: true } });

const { t } = useI18n();
const auth = useAuth();

const MAX_LENGTH = 2000; // keep in sync with backend comment.validation.js

const comments = ref([]);
const draft = ref("");
const editingId = ref("");
const editDraft = ref("");
const err = ref("");
const busy = ref(false);
const loaded = ref(false);

const base = computed(() => `/tests/${props.testId}/comments`);
const trimmed = computed(() => draft.value.trim());
const canSubmit = computed(() => trimmed.value.length > 0 && trimmed.value.length <= MAX_LENGTH && !busy.value);

function canModify(comment) {
  return comment.authorId === auth.user?.id;
}
// Author can delete own; privileged roles can moderate (delete any).
function canDelete(comment) {
  return canModify(comment) || auth.isAdmin;
}

// First letter of the author name, same initial-avatar look as the top-right UserMenu.
function initial(name) {
  return (name || "?").charAt(0).toUpperCase();
}

function formatDate(iso) {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

async function load() {
  err.value = "";
  try {
    comments.value = await api(base.value);
  } catch (e) {
    err.value = e.message;
  } finally {
    loaded.value = true;
  }
}

async function submit() {
  if (!canSubmit.value) return;
  busy.value = true;
  err.value = "";
  try {
    await api(base.value, { method: "POST", body: { body: trimmed.value } });
    draft.value = "";
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = false;
  }
}

function startEdit(comment) {
  editingId.value = comment.id;
  editDraft.value = comment.body;
}
function cancelEdit() {
  editingId.value = "";
  editDraft.value = "";
}
async function saveEdit(comment) {
  const body = editDraft.value.trim();
  if (!body || body.length > MAX_LENGTH) return;
  busy.value = true;
  err.value = "";
  try {
    await api(`${base.value}/${comment.id}`, { method: "PATCH", body: { body } });
    cancelEdit();
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove(comment) {
  if (!confirm(t("comments.confirmDelete"))) return;
  busy.value = true;
  err.value = "";
  try {
    await api(`${base.value}/${comment.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section class="comments card">
    <h3 class="comments-title">
      {{ $t("comments.title") }}
      <span v-if="loaded" class="count">{{ comments.length }}</span>
    </h3>

    <p v-if="err" class="error">{{ err }}</p>

    <!-- Composer: open to every authenticated user. -->
    <form class="composer" @submit.prevent="submit">
      <textarea
        v-model="draft"
        name="comment"
        :maxlength="MAX_LENGTH"
        :placeholder="$t('comments.placeholder')"
        rows="3"
      ></textarea>
      <div class="composer-foot">
        <span class="counter" :class="{ over: trimmed.length > MAX_LENGTH }">{{ trimmed.length }}/{{ MAX_LENGTH }}</span>
        <button type="submit" class="primary small" :disabled="!canSubmit">{{ $t("comments.submit") }}</button>
      </div>
    </form>

    <ul v-if="comments.length" class="list">
      <li v-for="c in comments" :key="c.id" class="item">
        <div class="item-head">
          <span class="avatar" aria-hidden="true">{{ initial(c.author?.name) }}</span>
          <span class="author">{{ c.author?.name || $t("comments.unknownAuthor") }}</span>
          <span class="time">{{ formatDate(c.createdAt) }}</span>
          <span v-if="c.updatedAt && c.updatedAt !== c.createdAt" class="edited">{{ $t("comments.edited") }}</span>
        </div>

        <template v-if="editingId === c.id">
          <textarea v-model="editDraft" name="comment-edit" :maxlength="MAX_LENGTH" rows="3"></textarea>
          <div class="item-actions">
            <button class="small" @click="cancelEdit">{{ $t("comments.cancel") }}</button>
            <button class="primary small" :disabled="busy || !editDraft.trim()" @click="saveEdit(c)">{{ $t("comments.save") }}</button>
          </div>
        </template>
        <template v-else>
          <!-- Text interpolation auto-escapes; pre-wrap keeps newlines without HTML. -->
          <p class="body">{{ c.body }}</p>
          <div class="item-actions" v-if="canModify(c) || canDelete(c)">
            <button v-if="canModify(c)" class="link" @click="startEdit(c)">{{ $t("comments.edit") }}</button>
            <button v-if="canDelete(c)" class="link danger" @click="remove(c)">{{ $t("comments.delete") }}</button>
          </div>
        </template>
      </li>
    </ul>
    <p v-else-if="loaded" class="muted empty">{{ $t("comments.empty") }}</p>
  </section>
</template>

<style scoped>
.comments { display: flex; flex-direction: column; gap: 14px; }
.comments-title { margin: 0; display: flex; align-items: center; gap: 8px; font-size: 16px; }
.count {
  font-size: 12px; font-weight: 600; color: var(--muted);
  background: var(--panel2); border: 1px solid var(--line); border-radius: 999px; padding: 1px 8px;
}
.composer { display: flex; flex-direction: column; gap: 8px; }
.composer textarea, .item textarea { width: 100%; resize: vertical; font: inherit; }
.composer-foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.counter { font-size: 12px; color: var(--muted); font-variant-numeric: tabular-nums; }
.counter.over { color: #e5534b; }
.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
.item { border-top: 1px solid var(--line); padding-top: 12px; display: flex; flex-direction: column; gap: 6px; }
.item-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
/* Same initial-avatar look as the top-right UserMenu. */
.avatar {
  width: 24px; height: 24px; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--accent); color: #04141d; font-size: 12px; font-weight: 800;
  flex: 0 0 auto;
}
.author { font-weight: 600; }
.time { font-size: 12px; color: var(--muted); }
.edited { font-size: 11px; color: var(--muted); font-style: italic; }
.body { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.item-actions { display: flex; gap: 10px; }
.link { background: none; border: none; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
.link:hover { color: var(--txt); }
.link.danger:hover { color: #e5534b; }
.small { padding: 4px 10px; font-size: .85rem; }
.empty { color: var(--muted); }
.error { color: #e5534b; margin: 0; }
</style>
