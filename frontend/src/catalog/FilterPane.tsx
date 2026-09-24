import type {CatalogFilters, CatalogQuery, TagOption} from '../shared/types';

interface FilterPaneProps {
  filters: CatalogFilters;
  query: CatalogQuery;
  onChange(field: keyof Omit<CatalogQuery, 'search'>, value: string): void;
}

const groups: Array<{
  field: keyof Omit<CatalogQuery, 'search'>;
  source: keyof CatalogFilters;
  title: string;
}> = [
  {field: 'stageId', source: 'stages', title: '学段'},
  {field: 'subjectId', source: 'subjects', title: '学科'},
  {field: 'editionId', source: 'editions', title: '版本'},
  {field: 'gradeId', source: 'grades', title: '年级'},
];

export function FilterPane({filters, query, onChange}: FilterPaneProps) {
  const stageGroup = groups[0];
  return (
    <aside className="filter-pane" aria-label="教材筛选">
      <div className="filter-heading"><strong>电子教材</strong></div>
      <FilterGroup
        title={stageGroup.title}
        name={stageGroup.field}
        options={filters[stageGroup.source]}
        value={query[stageGroup.field]}
        onChange={(value) => onChange(stageGroup.field, value)}
        variant="stages"
      />
    </aside>
  );
}

export function CatalogFilterRows({filters, query, onChange}: FilterPaneProps) {
  return (
    <div className="catalog-filter-rows" aria-label="教材条件筛选">
      {groups.slice(1).map((group) => (
        <FilterGroup
          key={group.field}
          title={group.title}
          name={group.field}
          options={filters[group.source]}
          value={query[group.field]}
          onChange={(value) => onChange(group.field, value)}
          variant="row"
        />
      ))}
    </div>
  );
}

function FilterGroup({
  title, name, options, value, onChange, variant,
}: {
  title: string;
  name: string;
  options: TagOption[];
  value: string;
  onChange(value: string): void;
  variant: 'stages' | 'row';
}) {
  return (
    <div className={`filter-group filter-group-${variant}`} role="group" aria-label={title}>
      <span className="filter-group-title">{title}</span>
      {options.map((option) => (
        <label key={option.id}>
          <input type="radio" name={name} checked={value === option.id} onChange={() => onChange(option.id)} />
          {option.name}
        </label>
      ))}
    </div>
  );
}
