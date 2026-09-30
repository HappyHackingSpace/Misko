import { ref } from "vue";
import { environments } from "../api/endpoints.js";

// A test pins one apparatus revision, but the API returns only its id. This maps
// every revision id to the apparatus it belongs to and its number, so a screen can
// say which measurements a test was run with. Revisions never change, so an id
// that is found stays correct; one that is missing was created after the load.
export function useEnvironmentRevisions() {
  const byId = ref({});

  async function load() {
    const list = (await environments.allRevisions()).data || [];
    byId.value = Object.fromEntries(
      list.map((revision) => [
        revision.id,
        { environmentId: revision.environmentId, environmentName: revision.environmentName, number: revision.number },
      ]),
    );
  }

  return { byId, load };
}
