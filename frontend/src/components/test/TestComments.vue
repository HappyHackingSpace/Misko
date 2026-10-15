<script setup>
// The discussion of a session. Every signed-in role may read and write; only
// the author edits, and the author or a lab manager deletes, as the API
// enforces. Bodies are plain text rendered by interpolation, never as HTML.
import { computed, onMounted, ref } from "vue";
import { NAlert, NButton, NCard, NInput, NPopconfirm, NSpace } from "naive-ui";
import { tests } from "../../api/tests.js";
import { useAuth } from "../../stores/auth.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";

const props = defineProps({ testId: { type: String, required: true } });
const auth = useAuth();
const fmt = useExecutionFormat();

const MAX_LENGTH = 2000;
const comments = ref([]);
const draft = ref("");
const editingId = ref("");
const editDraft = ref("");
const error = ref("");
const busy = ref(false);
const loaded = ref(false);

const trimmed = computed(() => draft.value.trim());
const canEdit = (comment) => comment.authorId === auth.user?.id;
const canDelete = (comment) => canEdit(comment) || ["SUPERADMIN", "LAB_MANAGER"].includes(auth.user?.role);

async function load() {
  try {
    comments.value = (await tests.comments(props.testId)).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loaded.value = true;
  }
}

async function run(action) {
  busy.value = true;
  error.value = "";
  try {
    await action();
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

const submit = () =>
  run(async () => {
    await tests.createComment(props.testId, trimmed.value);
    draft.value = "";
  });

const saveEdit = (comment) =>
  run(async () => {
    await tests.updateComment(props.testId, comment.id, editDraft.value.trim());
    editingId.value = "";
  });

const remove = (comment) => run(() => tests.deleteComment(props.testId, comment.id));

function startEdit(comment) {
  editingId.value = comment.id;
  editDraft.value = comment.body;
}

onMounted(load);
</script>

<template>
  <NCard size="small" :bordered="true" data-test="comments">
    <template #header>
      {{ $t("comments.title") }} <span v-if="loaded" class="muted" data-test="comment-count">({{ comments.length }})</span>
    </template>
    <div class="thread">
      <div v-for="c in comments" :key="c.id" class="item" data-test="comment" :data-comment="c.id">
        <div class="head">
          <strong>{{ c.authorName || $t("comments.unknownAuthor") }}</strong>
          <span class="muted small">{{ fmt.dateTime(c.createdAt) }}</span>
          <span v-if="c.edited" class="muted small" data-test="comment-edited">{{ $t("comments.edited") }}</span>
        </div>
        <template v-if="editingId === c.id">
          <NInput v-model:value="editDraft" type="textarea" :maxlength="MAX_LENGTH" :autosize="{ minRows: 2, maxRows: 8 }" :input-props="{ 'data-test': 'comment-edit-draft' }" />
          <NSpace :size="8">
            <NButton size="small" @click="editingId = ''">{{ $t("comments.cancel") }}</NButton>
            <NButton size="small" type="primary" :disabled="busy || !editDraft.trim()" data-test="comment-edit-save" @click="saveEdit(c)">
              {{ $t("comments.save") }}
            </NButton>
          </NSpace>
        </template>
        <template v-else>
          <p class="body" data-test="comment-body">{{ c.body }}</p>
          <NSpace v-if="canEdit(c) || canDelete(c)" :size="4">
            <NButton v-if="canEdit(c)" text size="small" data-test="comment-edit" @click="startEdit(c)">{{ $t("comments.edit") }}</NButton>
            <NPopconfirm v-if="canDelete(c)" @positive-click="remove(c)">
              <template #trigger>
                <NButton text size="small" type="error" data-test="comment-delete">{{ $t("comments.delete") }}</NButton>
              </template>
              {{ $t("comments.confirmDelete") }}
            </NPopconfirm>
          </NSpace>
        </template>
      </div>
      <p v-if="loaded && !comments.length" class="muted" data-test="comments-empty">{{ $t("comments.empty") }}</p>

      <NAlert v-if="error" type="error" :title="error" data-test="comment-error" />
      <form class="composer" data-test="comment-form" @submit.prevent="submit">
        <NInput
          v-model:value="draft"
          type="textarea"
          :maxlength="MAX_LENGTH"
          show-count
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="$t('comments.placeholder')"
          :input-props="{ 'data-test': 'comment-draft' }"
        />
        <NButton type="primary" size="small" attr-type="submit" :disabled="busy || !trimmed" data-test="comment-submit">{{ $t("comments.submit") }}</NButton>
      </form>
    </div>
  </NCard>
</template>

<style scoped>
.thread { display: flex; flex-direction: column; gap: 12px; }
.item { display: flex; flex-direction: column; gap: 4px; padding-bottom: 10px; border-bottom: 1px solid var(--line); }
.head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.small { font-size: 12px; }
.body { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.composer { display: flex; flex-direction: column; align-items: flex-end; gap: 8px; }
</style>
