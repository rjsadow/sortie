import { useState, useMemo, useCallback, useEffect } from 'react';
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
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
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

// Map API response to ApplicationTemplate format
function mapApiTemplateToApplicationTemplate(apiTemplate: Record<string, unknown>): ApplicationTemplate {
  return {
    template_id: apiTemplate.template_id as string,
    template_version: apiTemplate.template_version as string,
    template_category: apiTemplate.template_category as TemplateCategory,
    name: apiTemplate.name as string,
    description: apiTemplate.description as string,
    url: (apiTemplate.url as string) || '',
    icon: (apiTemplate.icon as string) || '',
    category: apiTemplate.category as string,
    launch_type: apiTemplate.launch_type as 'url' | 'container' | 'web_proxy',
    container_image: apiTemplate.container_image as string | undefined,
    container_port: apiTemplate.container_port as number | undefined,
    container_args: apiTemplate.container_args as string[] | undefined,
    tags: (apiTemplate.tags as string[]) || [],
    maintainer: apiTemplate.maintainer as string | undefined,
    documentation_url: apiTemplate.documentation_url as string | undefined,
    recommended_limits: apiTemplate.recommended_limits as ApplicationTemplate['recommended_limits'],
  };
}

export function useTemplates(): UseTemplatesReturn {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<TemplateCategory | 'all'>('all');
  const [templates, setTemplates] = useState<ApplicationTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [catalogVersion, setCatalogVersion] = useState('1.0.0');

  const fetchTemplates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch('/api/templates');
      if (!response.ok) {
        throw new Error('Failed to fetch templates');
      }
      const data = await response.json();
      // Map API response to ApplicationTemplate format
      const mappedTemplates = data.map(mapApiTemplateToApplicationTemplate);
      setTemplates(mappedTemplates);
      setCatalogVersion('1.0.0'); // API doesn't return version, use default
    } catch (err) {
      console.warn('Failed to fetch templates from API, falling back to static data:', err);
      setError(err instanceof Error ? err.message : 'Unknown error');
      // Fallback to static JSON
      const catalog = templateData as TemplateCatalog;
      setTemplates(catalog.templates);
      setCatalogVersion(catalog.version);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

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
    loading,
    error,
    refetch: fetchTemplates,
  };
}

export { CATEGORY_LABELS };
