<script setup>
import { ref, computed, onMounted } from "vue";
import { tests } from "../api/endpoints.js";
import { useAuth } from "../stores/auth.js";

// Discussion thread for a single test. Reading and writing a comment need only
// read access, so every role including a viewer can take part; editing is the
// author's alone and deleting is the author's or a lab manager's, which is what
// the API enforces. Bodies are rendered with text interpolation only (never
// v-html), so stored content is escaped by Vue and cannot execute.
const props = defineProps({ testId: { type: String, required: true } });

const auth = useAuth();

// The API refuses an empty comment and anything over 2000 characters.
const MAX_LENGTH = 2000;

const comments = ref([]);
const draft = ref("");
const editingId = ref("");
const editDraft = ref("");
const err = ref("");
const busy = ref(false);
const loaded = ref(false);
const confirmingId = ref("");

const trimmed = computed(() => draft.value.trim());
const canSubmit = computed(() => trimmed.value.length > 0 && trimmed.value.length <= MAX_LENGTH && !busy.value);

// Only the author may edit. Deleting is also open to a role that moderates,
// which is the same set the API calls privileged: the roles that manage users.
const canEdit = (comment) => comment.authorId === auth.user?.id;
const canDelete = (comment) => canEdit(comment) || auth.canManageUsers;

const initial = (name) => (name || "?").charAt(0).toUpperCase();

const formatDate = (iso) => {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
};

async function load() {
  err.value = "";
  try {
    comments.value = (await tests.comments(props.testId)).data || [];
  } catch (e) {
    err.value = e.message;
  } finally {
    loaded.value = true;
  }
}

async function run(action) {
  busy.value = true;
  err.value = "";
  try {
    await action();
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function submit() {
  if (!canSubmit.value) return;
  await run(async () => {
    await tests.createComment(props.testId, trimmed.value);
    draft.value = "";
  });
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
  await run(async () => {
    await tests.updateComment(props.testId, comment.id, body);
    cancelEdit();
  });
}

// Deleting a comment cannot be undone, so it is confirmed in the thread itself
// rather than in a browser dialog: the confirmation is part of the page.
async function remove(comment) {
  await run(async () => {
    await tests.deleteComment(props.testId, comment.id);
    confirmingId.value = "";
  });
}

onMounted(load);
</script>

<template>
  <section class="comments card" data-test="comments">
    <h3 class="comments-title">
      {{ $t("comments.title") }}
      <span v-if="loaded" class="count" data-test="comment-count">{{ comments.length }}</span>
    </h3>

    <p v-if="err" class="error" data-test="comment-error">{{ err }}</p>

    <!-- Writing a comment is open to every signed-in role, viewers included. -->
    <form class="composer" data-test="comment-form" @submit.prevent="submit">
      <textarea
        v-model="draft"
        name="comment"
        :maxlength="MAX_LENGTH"
        :placeholder="$t('comments.placeholder')"
        data-test="comment-draft"
        rows="3"
      ></textarea>
      <div class="composer-foot">
        <span class="counter" :class="{ over: trimmed.length > MAX_LENGTH }">{{ trimmed.length }}/{{ MAX_LENGTH }}</span>
        <button type="submit" class="primary small" :disabled="!canSubmit" data-test="comment-submit">
          {{ $t("comments.submit") }}
        </button>
      </div>
    </form>

    <ul v-if="comments.length" class="list">
      <!-- The row carries its own id: while a comment is being edited its body
           is a textarea, so text is no longer what identifies it. -->
      <li v-for="c in comments" :key="c.id" class="item" data-test="comment" :data-comment="c.id" :data-author="c.authorId">
        <div class="item-head">
          <span class="avatar" aria-hidden="true">{{ initial(c.authorName) }}</span>
          <span class="author">{{ c.authorName || $t("comments.unknownAuthor") }}</span>
          <span class="time">{{ formatDate(c.createdAt) }}</span>
          <!-- The API says whether a comment was edited; the panel does not
               infer it from timestamps. -->
          <span v-if="c.edited" class="edited" data-test="comment-edited">{{ $t("comments.edited") }}</span>
        </div>

        <template v-if="editingId === c.id">
          <textarea v-model="editDraft" name="comment-edit" :maxlength="MAX_LENGTH" data-test="comment-edit-draft" rows="3"></textarea>
          <div class="item-actions">
            <button class="small" data-test="comment-edit-cancel" @click="cancelEdit">{{ $t("comments.cancel") }}</button>
            <button
              class="primary small"
              :disabled="busy || !editDraft.trim()"
              data-test="comment-edit-save"
              @click="saveEdit(c)"
            >
              {{ $t("comments.save") }}
            </button>
          </div>
        </template>
        <template v-else>
          <!-- Text interpolation auto-escapes; pre-wrap keeps newlines without HTML. -->
          <p class="body" data-test="comment-body">{{ c.body }}</p>
          <div class="item-actions" v-if="canEdit(c) || canDelete(c)">
            <button v-if="canEdit(c)" class="link" data-test="comment-edit" @click="startEdit(c)">
              {{ $t("comments.edit") }}
            </button>
            <template v-if="canDelete(c)">
              <button v-if="confirmingId !== c.id" class="link danger" data-test="comment-delete" @click="confirmingId = c.id">
                {{ $t("comments.delete") }}
              </button>
              <template v-else>
                <span class="muted" data-test="comment-confirm">{{ $t("comments.confirmDelete") }}</span>
                <button class="link danger" :disabled="busy" data-test="comment-delete-confirm" @click="remove(c)">
                  {{ $t("comments.delete") }}
                </button>
                <button class="link" data-test="comment-delete-cancel" @click="confirmingId = ''">
                  {{ $t("comments.cancel") }}
                </button>
              </template>
            </template>
          </div>
        </template>
      </li>
    </ul>
    <p v-else-if="loaded" class="muted empty" data-test="comments-empty">{{ $t("comments.empty") }}</p>
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
.item-actions { display: flex; gap: 10px; align-items: center; }
.link { background: none; border: none; padding: 0; font-size: 13px; color: var(--muted); cursor: pointer; }
.link:hover { color: var(--txt); }
.link.danger:hover { color: #e5534b; }
.small { padding: 4px 10px; font-size: .85rem; }
.empty { color: var(--muted); }
.error { color: #e5534b; margin: 0; }
</style>
