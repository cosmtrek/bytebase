<template>
  <div class="save-sheet-modal w-80">
    <NInput
      ref="sheetNameInputRef"
      v-model:value="sheetName"
      :placeholder="$t('sql-editor.save-sheet-input-placeholder')"
      @keyup.enter="(e: Event) => emit('save-sheet', sheetName)"
    />
  </div>
  <div class="mt-4 flex justify-end space-x-2">
    <NButton @click="(e: Event) => emit('close')">{{
      $t("common.close")
    }}</NButton>
    <NButton type="primary" @click="emit('save-sheet', sheetName)">
      {{ $t("common.save") }}
    </NButton>
  </div>
</template>

<script lang="ts" setup>
import { ref, nextTick, defineEmits } from "vue";
import { useNamespacedGetters } from "vuex-composition-helpers";
import { TabGetters } from "../../../types";

const emit = defineEmits<{
  (e: "close"): void;
  (e: "save-sheet", content: string): void;
}>();

const { currentTab } = useNamespacedGetters<TabGetters>("tab", ["currentTab"]);

const sheetName = ref(currentTab.value.name);
const sheetNameInputRef = ref();

nextTick(() => {
  sheetNameInputRef.value?.focus();
});
</script>
