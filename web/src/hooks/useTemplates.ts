import { useState, useMemo, useCallback } from 'react';
import type { Application, ApplicationTemplate, TemplateCategory, TemplateCatalog } from '../types';
import templateData from '../data/templates.json';

interface UseTemplatesReturn {
  templates: ApplicationTemplate[];
  categories: TemplateCategory[];
  searchQuery: string;
  setSearchQuery: (query: string) => void;
  selectedCategory: TemplateCategory | 'all';
  setSelectedCategory: (category: TemplateCategory | 'all') => void;
  filteredTemplates: ApplicationTemplate[];
  templateToApplication: (template: ApplicationTemplate, customId?: string) => Application;
  catalogVersion: string;
}

const CATEGORY_LABELS: Record<TemplateCategory, string> = {
  development: 'Development',
  productivity: 'Productivity',
  communication: 'Communication',
  browsers: 'Browsers',
  monitoring: 'Monitoring',
  databases: 'Databases',
  creative: 'Creative',
};

export function useTemplates(): UseTemplatesReturn {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<TemplateCategory | 'all'>('all');

  const catalog = templateData as TemplateCatalog;
  const templates = catalog.templates;
  const catalogVersion = catalog.version;

  const categories = useMemo(() => {
    const uniqueCategories = [...new Set(templates.map((t) => t.template_category))];
    return uniqueCategories.sort((a, b) => {
      const order: TemplateCategory[] = [
        'development',
        'productivity',
        'communication',
        'browsers',
        'monitoring',
        'databases',
        'creative',
      ];
      return order.indexOf(a) - order.indexOf(b);
    });
  }, [templates]);

  const filteredTemplates = useMemo(() => {
    let result = templates;

    if (selectedCategory !== 'all') {
      result = result.filter((t) => t.template_category === selectedCategory);
    }

    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(
        (t) =>
          t.name.toLowerCase().includes(query) ||
          t.description.toLowerCase().includes(query) ||
          t.tags.some((tag) => tag.toLowerCase().includes(query))
      );
    }

    return result;
  }, [templates, selectedCategory, searchQuery]);

  const templateToApplication = useCallback(
    (template: ApplicationTemplate, customId?: string): Application => {
      const id = customId || `app-${template.template_id}-${Date.now()}`;
      return {
        id,
        name: template.name,
        description: template.description,
        url: template.url,
        icon: template.icon,
        category: template.category,
        launch_type: template.launch_type,
        container_image: template.container_image,
        resource_limits: template.recommended_limits,
      };
    },
    []
  );

  return {
    templates,
    categories,
    searchQuery,
    setSearchQuery,
    selectedCategory,
    setSelectedCategory,
    filteredTemplates,
    templateToApplication,
    catalogVersion,
  };
}

export { CATEGORY_LABELS };
