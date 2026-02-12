import { useState, useEffect } from 'react';
import type { Category } from '../types';
import {
  listCategories,
  createCategory,
  updateCategory,
  deleteCategory,
  listCategoryAdmins,
  addCategoryAdmin,
  removeCategoryAdmin,
  listCategoryApprovedUsers,
  addCategoryApprovedUser,
  removeCategoryApprovedUser,
  listUsersBasic,
} from '../services/auth';

interface CategoryManagerProps {
  darkMode: boolean;
  isSystemAdmin: boolean;
  adminCategoryIds: string[];
}

export function CategoryManager({ darkMode, isSystemAdmin, adminCategoryIds }: CategoryManagerProps) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [users, setUsers] = useState<{ id: string; username: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  // Form state
  const [showForm, setShowForm] = useState(false);
  const [editingCategory, setEditingCategory] = useState<Category | null>(null);
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');

  // Detail panel state
  const [selectedCategory, setSelectedCategory] = useState<Category | null>(null);
  const [categoryAdmins, setCategoryAdmins] = useState<string[]>([]);
  const [approvedUsers, setApprovedUsers] = useState<string[]>([]);
  const [addAdminUserId, setAddAdminUserId] = useState('');
  const [addApprovedUserId, setAddApprovedUserId] = useState('');

  const cardBg = darkMode ? 'bg-gray-800' : 'bg-white';
  const textColor = darkMode ? 'text-gray-100' : 'text-gray-900';
  const mutedText = darkMode ? 'text-gray-400' : 'text-gray-600';
  const inputBg = darkMode ? 'bg-gray-700 border-gray-600' : 'bg-white border-gray-300';
  const inputText = darkMode ? 'text-gray-100' : 'text-gray-900';

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    try {
      const [catList, userList] = await Promise.all([listCategories(), listUsersBasic()]);
      const filtered = isSystemAdmin ? catList : catList.filter((c) => adminCategoryIds.includes(c.id));
      setCategories(filtered);
      setUsers(userList);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load categories');
    } finally {
      setLoading(false);
    }
  };

  const openForm = (cat?: Category) => {
    if (cat) {
      setEditingCategory(cat);
      setFormName(cat.name);
      setFormDescription(cat.description);
    } else {
      setEditingCategory(null);
      setFormName('');
      setFormDescription('');
    }
    setShowForm(true);
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingCategory(null);
  };

  const handleSave = async () => {
    setError('');
    if (!formName) {
      setError('Category name is required');
      return;
    }
    try {
      if (editingCategory) {
        await updateCategory(editingCategory.id, {
          name: formName,
          description: formDescription,
        });
        setSuccess('Category updated');
      } else {
        const id = `cat-${formName.toLowerCase().replace(/\s+/g, '-')}-${Date.now()}`;
        await createCategory({
          id,
          name: formName,
          description: formDescription,
        });
        setSuccess('Category created');
      }
      closeForm();
      await loadData();
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save category');
    }
  };

  const handleDelete = async (cat: Category) => {
    if (!confirm(`Delete category "${cat.name}"? Apps in this category will become uncategorized.`)) return;
    setError('');
    try {
      await deleteCategory(cat.id);
      if (selectedCategory?.id === cat.id) setSelectedCategory(null);
      await loadData();
      setSuccess('Category deleted');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete category');
    }
  };

  const selectCategory = async (cat: Category) => {
    setSelectedCategory(cat);
    setAddAdminUserId('');
    setAddApprovedUserId('');
    try {
      const [admins, approved] = await Promise.all([
        listCategoryAdmins(cat.id),
        listCategoryApprovedUsers(cat.id),
      ]);
      setCategoryAdmins(admins);
      setApprovedUsers(approved);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load category details');
    }
  };

  const handleAddAdmin = async () => {
    if (!selectedCategory || !addAdminUserId) return;
    setError('');
    try {
      await addCategoryAdmin(selectedCategory.id, addAdminUserId);
      setAddAdminUserId('');
      setCategoryAdmins([...categoryAdmins, addAdminUserId]);
      setSuccess('Admin added');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add admin');
    }
  };

  const handleRemoveAdmin = async (userId: string) => {
    if (!selectedCategory) return;
    setError('');
    try {
      await removeCategoryAdmin(selectedCategory.id, userId);
      setCategoryAdmins(categoryAdmins.filter((id) => id !== userId));
      setSuccess('Admin removed');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove admin');
    }
  };

  const handleAddApproved = async () => {
    if (!selectedCategory || !addApprovedUserId) return;
    setError('');
    try {
      await addCategoryApprovedUser(selectedCategory.id, addApprovedUserId);
      setAddApprovedUserId('');
      setApprovedUsers([...approvedUsers, addApprovedUserId]);
      setSuccess('Approved user added');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add approved user');
    }
  };

  const handleRemoveApproved = async (userId: string) => {
    if (!selectedCategory) return;
    setError('');
    try {
      await removeCategoryApprovedUser(selectedCategory.id, userId);
      setApprovedUsers(approvedUsers.filter((id) => id !== userId));
      setSuccess('Approved user removed');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove approved user');
    }
  };

  const getUsernameById = (id: string) => {
    const user = users.find((u) => u.id === id);
    return user ? user.username : id;
  };

  // Filter users not already in a list for the dropdown
  const availableAdminUsers = users.filter((u) => !categoryAdmins.includes(u.id));
  const availableApprovedUsers = users.filter((u) => !approvedUsers.includes(u.id));

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-brand-accent"></div>
      </div>
    );
  }

  return (
    <div>
      {error && (
        <div className="mb-4 p-3 bg-red-500/20 border border-red-500 rounded-lg text-red-500">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-4 p-3 bg-green-500/20 border border-green-500 rounded-lg text-green-500">
          {success}
        </div>
      )}

      <div>
        {/* Category List */}
        <div className={`${cardBg} rounded-lg p-6`}>
          <div className="flex justify-between items-center mb-4">
            <h2 className={`text-lg font-semibold ${textColor}`}>Categories</h2>
            {isSystemAdmin && (
              <button
                onClick={() => openForm()}
                className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
              >
                Create Category
              </button>
            )}
          </div>

          {/* Category Form Modal */}
          {showForm && (
            <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
              <div className={`${cardBg} rounded-lg p-6 max-w-md w-full mx-4`}>
                <h3 className={`text-lg font-semibold mb-4 ${textColor}`}>
                  {editingCategory ? 'Edit Category' : 'Create Category'}
                </h3>
                <div className="space-y-4">
                  <div>
                    <label className={`block text-sm mb-1 ${mutedText}`}>Name *</label>
                    <input
                      type="text"
                      value={formName}
                      onChange={(e) => setFormName(e.target.value)}
                      placeholder="e.g., Development"
                      className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                    />
                  </div>
                  <div>
                    <label className={`block text-sm mb-1 ${mutedText}`}>Description</label>
                    <textarea
                      value={formDescription}
                      onChange={(e) => setFormDescription(e.target.value)}
                      placeholder="Brief description"
                      rows={2}
                      className={`w-full px-3 py-2 rounded-lg border ${inputBg} ${inputText}`}
                    />
                  </div>
                </div>
                <div className="flex justify-end gap-3 mt-6">
                  <button
                    onClick={closeForm}
                    className={`px-4 py-2 rounded-lg ${darkMode ? 'bg-gray-700 hover:bg-gray-600' : 'bg-gray-200 hover:bg-gray-300'} ${textColor}`}
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleSave}
                    className="px-4 py-2 bg-brand-accent text-white rounded-lg hover:bg-brand-primary transition-colors"
                  >
                    {editingCategory ? 'Update' : 'Create'}
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Category Table */}
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                  <th className={`text-left py-2 ${mutedText}`}>Name</th>
                  <th className={`text-right py-2 ${mutedText}`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {categories.map((cat) => (
                  <tr
                    key={cat.id}
                    className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}
                  >
                    <td className={`py-3 ${textColor}`}>
                      <div className="font-medium">{cat.name}</div>
                      {cat.description && (
                        <div className={`text-xs ${mutedText}`}>{cat.description}</div>
                      )}
                    </td>
                    <td className="py-3 text-right">
                      <button
                        onClick={() => selectCategory(cat)}
                        className="text-brand-accent hover:text-brand-primary text-sm mr-3"
                      >
                        Manage
                      </button>
                      <button
                        onClick={() => openForm(cat)}
                        className="text-blue-500 hover:text-blue-400 text-sm mr-3"
                      >
                        Edit
                      </button>
                      {isSystemAdmin && (
                        <button
                          onClick={() => handleDelete(cat)}
                          className="text-red-500 hover:text-red-400 text-sm"
                        >
                          Delete
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {categories.length === 0 && (
              <p className={`text-center py-8 ${mutedText}`}>
                No categories found. Click "Create Category" to add one.
              </p>
            )}
          </div>
        </div>

        {/* Manage Users Modal */}
        {selectedCategory && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className={`${cardBg} rounded-lg p-6 max-w-lg w-full mx-4`}>
              <div className="flex justify-between items-center mb-4">
                <h3 className={`text-lg font-semibold ${textColor}`}>Manage: {selectedCategory.name}</h3>
                <button
                  onClick={() => setSelectedCategory(null)}
                  className={`${mutedText} hover:${textColor}`}
                >
                  &#x2715;
                </button>
              </div>

              <div className="grid grid-cols-2 gap-6">
                {/* Category Admins */}
                <div>
                  <h4 className={`text-sm font-medium mb-2 ${textColor}`}>Category Admins</h4>
                  <div className="space-y-1 mb-2">
                    {categoryAdmins.map((uid) => (
                      <div key={uid} className={`flex items-center justify-between text-sm ${mutedText}`}>
                        <span>{getUsernameById(uid)}</span>
                        <button
                          onClick={() => handleRemoveAdmin(uid)}
                          className="text-red-500 hover:text-red-400 text-xs"
                        >
                          Remove
                        </button>
                      </div>
                    ))}
                    {categoryAdmins.length === 0 && (
                      <p className={`text-xs ${mutedText}`}>No category admins</p>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <select
                      value={addAdminUserId}
                      onChange={(e) => setAddAdminUserId(e.target.value)}
                      className={`flex-1 px-2 py-1 text-sm rounded border ${inputBg} ${inputText}`}
                    >
                      <option value="">Select user...</option>
                      {availableAdminUsers.map((u) => (
                        <option key={u.id} value={u.id}>{u.username}</option>
                      ))}
                    </select>
                    <button
                      onClick={handleAddAdmin}
                      disabled={!addAdminUserId}
                      className="px-3 py-1 text-sm bg-brand-accent text-white rounded hover:bg-brand-primary disabled:opacity-50 transition-colors"
                    >
                      Add
                    </button>
                  </div>
                </div>

                {/* Approved Users */}
                <div>
                  <h4 className={`text-sm font-medium mb-2 ${textColor}`}>Approved Users</h4>
                  <div className="space-y-1 mb-2">
                    {approvedUsers.map((uid) => (
                      <div key={uid} className={`flex items-center justify-between text-sm ${mutedText}`}>
                        <span>{getUsernameById(uid)}</span>
                        <button
                          onClick={() => handleRemoveApproved(uid)}
                          className="text-red-500 hover:text-red-400 text-xs"
                        >
                          Remove
                        </button>
                      </div>
                    ))}
                    {approvedUsers.length === 0 && (
                      <p className={`text-xs ${mutedText}`}>No approved users</p>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <select
                      value={addApprovedUserId}
                      onChange={(e) => setAddApprovedUserId(e.target.value)}
                      className={`flex-1 px-2 py-1 text-sm rounded border ${inputBg} ${inputText}`}
                    >
                      <option value="">Select user...</option>
                      {availableApprovedUsers.map((u) => (
                        <option key={u.id} value={u.id}>{u.username}</option>
                      ))}
                    </select>
                    <button
                      onClick={handleAddApproved}
                      disabled={!addApprovedUserId}
                      className="px-3 py-1 text-sm bg-brand-accent text-white rounded hover:bg-brand-primary disabled:opacity-50 transition-colors"
                    >
                      Add
                    </button>
                  </div>
                </div>
              </div>

              <div className="flex justify-end mt-6">
                <button
                  onClick={() => setSelectedCategory(null)}
                  className={`px-4 py-2 rounded-lg ${darkMode ? 'bg-gray-700 hover:bg-gray-600' : 'bg-gray-200 hover:bg-gray-300'} ${textColor}`}
                >
                  Done
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
