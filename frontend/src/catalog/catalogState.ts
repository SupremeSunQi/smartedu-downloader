import type {CatalogFilters, CatalogQuery, TagOption, Textbook} from '../shared/types';

export const emptyQuery: CatalogQuery = {
  search: '', stageId: '', subjectId: '', editionId: '', gradeId: '', volumeId: '',
};

const filterOrder: Array<keyof Omit<CatalogQuery, 'search'>> = [
  'stageId', 'subjectId', 'editionId', 'gradeId', 'volumeId',
];

const displayOrder = {
  stages: ['小学', '初中', '小学（五·四学制）', '初中（五·四学制）', '高中', '特殊教育'],
  subjects: ['道德与法治', '语文', '数学', '英语', '艺术·音乐', '艺术·美术', '科学', '语文·书法练习指导', '体育与健康', '信息科技'],
  editions: ['统编版'],
  grades: ['一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '七年级', '八年级', '九年级', '高一', '高二', '高三', '学生读本'],
};

export function normalizeStageName(name: string): string {
  return name.replace(/[•・]/g, '·').replace(/[()]/g, (character) => character === '(' ? '（' : '）');
}

export function selectAvailableQuery(books: Textbook[], query: CatalogQuery = emptyQuery): CatalogQuery {
  const next = {...query, search: '', volumeId: ''};
  const fields: Array<[keyof Pick<CatalogQuery, 'stageId' | 'subjectId' | 'editionId' | 'gradeId'>, keyof Pick<CatalogFilters, 'stages' | 'subjects' | 'editions' | 'grades'>]> = [
    ['stageId', 'stages'], ['subjectId', 'subjects'], ['editionId', 'editions'], ['gradeId', 'grades'],
  ];
  for (const [field, source] of fields) {
    const available = deriveFilters(books, next)[source];
    if (!available.some((option) => option.id === next[field])) next[field] = available[0]?.id ?? '';
  }
  return next;
}

export function filterCachedBooks(books: Textbook[], query: CatalogQuery): Textbook[] {
  return books.filter((book) => filterOrder.every((field) => !query[field] || book[field] === query[field]));
}

export function updateFilter(
  query: CatalogQuery,
  field: keyof Omit<CatalogQuery, 'search'>,
  value: string,
): CatalogQuery {
  const next = {...query, [field]: value};
  const changedAt = filterOrder.indexOf(field);
  for (const child of filterOrder.slice(changedAt + 1)) next[child] = '';
  return next;
}

export function deriveFilters(books: Textbook[], query: CatalogQuery): CatalogFilters {
  const stages = ordered(options(books, 'stageId', 'stage', normalizeStageName), displayOrder.stages);
  const subjects = ordered(options(matching(books, query, ['stageId']), 'subjectId', 'subject'), displayOrder.subjects);
  const editions = ordered(options(matching(books, query, ['stageId', 'subjectId']), 'editionId', 'edition'), displayOrder.editions);
  const grades = ordered(options(matching(books, query, ['stageId', 'subjectId', 'editionId']), 'gradeId', 'grade'), displayOrder.grades);
  const volumes = options(matching(books, query, ['stageId', 'subjectId', 'editionId', 'gradeId']), 'volumeId', 'volume');
  return {stages, subjects, editions, grades, volumes};
}

function ordered(options: TagOption[], order: string[]): TagOption[] {
  return options.sort((left, right) => {
    const leftIndex = order.indexOf(left.name);
    const rightIndex = order.indexOf(right.name);
    if (leftIndex !== rightIndex) return (leftIndex < 0 ? order.length : leftIndex) - (rightIndex < 0 ? order.length : rightIndex);
    return left.name.localeCompare(right.name, 'zh-CN');
  });
}

function matching(
  books: Textbook[],
  query: CatalogQuery,
  fields: Array<keyof Omit<CatalogQuery, 'search'>>,
): Textbook[] {
  return books.filter((book) => fields.every((field) => !query[field] || book[field] === query[field]));
}

function options(books: Textbook[], idField: keyof Textbook, nameField: keyof Textbook, normalizeName?: (name: string) => string): TagOption[] {
  const found = new Map<string, string>();
  for (const book of books) {
    const id = book[idField];
    const name = book[nameField];
    if (typeof id === 'string' && id && typeof name === 'string' && name && name !== '全部' && name !== '全部教材') found.set(id, normalizeName ? normalizeName(name) : name);
  }
  return [...found].map(([id, name]) => ({id, name}));
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '大小未知';
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
