import { useState } from "react";
import type { ConfigResponse } from "../api/schemas";

interface ConfigViewProps {
	data: ConfigResponse | null;
	isLoading: boolean;
	error: Error | null;
	onResourceSelect?: (resourceId: string) => void;
}

function ModuleCard({
	module,
	onResourceSelect: _,
}: {
	module: any;
	onResourceSelect?: (resourceId: string) => void;
}) {
	const [expanded, setExpanded] = useState(false);

	return (
		<div className="bg-white border border-gray-200 rounded-lg p-4">
			<div className="flex items-center justify-between cursor-pointer" onClick={() => setExpanded(!expanded)}>
				<div className="flex items-center gap-3">
					<span className="text-xl">📦</span>
					<div>
						<h3 className="font-semibold text-gray-900">{module.name}</h3>
						<p className="text-sm text-gray-600">{module.source}</p>
						{module.path && <p className="text-xs text-gray-500">{module.path}</p>}
					</div>
				</div>
				<span className={`text-gray-400 transition-transform ${expanded ? "rotate-90" : ""}`}>▶</span>
			</div>

			{expanded && module.calls && module.calls.length > 0 && (
				<div className="mt-4 pl-8 border-l-2 border-gray-100">
					<h4 className="text-sm font-medium text-gray-700 mb-2">Module Calls:</h4>
					<div className="space-y-2">
						{module.calls.map((call: any, index: number) => (
							<div key={index} className="bg-gray-50 p-2 rounded text-sm">
								<div className="font-medium">{call.name}</div>
								<div className="text-gray-600 text-xs">{call.source}</div>
								{call.dependencies && call.dependencies.length > 0 && (
									<div className="mt-1">
										<span className="text-xs text-gray-500">Depends on: </span>
										<span className="text-xs text-red-600">{call.dependencies.join(", ")}</span>
									</div>
								)}
							</div>
						))}
					</div>
				</div>
			)}
		</div>
	);
}

function ResourceCard({
	resource,
	onResourceSelect,
}: {
	resource: any;
	onResourceSelect?: (resourceId: string) => void;
}) {
	const isDataSource = resource.mode === "DataResourceMode";
	const hasExplicitDeps = resource.dependencies.explicit.length > 0;
	const hasImplicitDeps = resource.dependencies.implicit.length > 0;

	return (
		<div className="bg-white border border-gray-200 rounded-lg p-4 hover:border-gray-300 transition-colors cursor-pointer" onClick={() => onResourceSelect?.(resource.id)}>
			<div className="flex items-start gap-3">
				<span className="text-xl">{isDataSource ? "📖" : "🏗️"}</span>
				<div className="flex-1 min-w-0">
					<h3 className="font-semibold text-gray-900 truncate">{resource.name}</h3>
					<p className="text-sm text-gray-600">{resource.type}</p>
					<p className="text-xs text-gray-500 mt-1">{isDataSource ? "Data Source" : "Managed Resource"}</p>

					{(hasExplicitDeps || hasImplicitDeps) && (
						<div className="mt-2 space-y-1">
							{hasExplicitDeps && (
								<div className="flex items-center gap-1">
									<span className="text-xs text-red-600">🔗 {resource.dependencies.explicit.length} explicit</span>
								</div>
							)}
							{hasImplicitDeps && (
								<div className="flex items-center gap-1">
									<span className="text-xs text-blue-600">⚡ {resource.dependencies.implicit.length} implicit</span>
								</div>
							)}
						</div>
					)}
				</div>
			</div>
		</div>
	);
}

export default function ConfigView({ data, isLoading, error, onResourceSelect }: ConfigViewProps) {
	const [activeTab, setActiveTab] = useState<"modules" | "resources" | "providers">("modules");

	if (error) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="error-message max-w-md text-center">
					<h3 className="text-lg font-semibold mb-2">Configuration Error</h3>
					<p>Failed to load configuration data.</p>
					<p className="text-sm mt-2">{error.message}</p>
				</div>
			</div>
		);
	}

	if (isLoading) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="text-center">
					<div className="loading-spinner mx-auto mb-4" />
					<p className="text-gray-600">Loading configuration...</p>
				</div>
			</div>
		);
	}

	if (!data) {
		return (
			<div className="h-full flex items-center justify-center">
				<div className="text-center text-gray-500">
					<div className="text-4xl mb-4">📄</div>
					<h3 className="text-lg font-semibold mb-2">No Configuration Data</h3>
					<p>No configuration data available.</p>
				</div>
			</div>
		);
	}

	return (
		<div className="h-full flex flex-col">
			{/* Tab navigation */}
			<div className="flex border-b border-gray-200 bg-white">
				<button
					type="button"
					onClick={() => setActiveTab("modules")}
					className={`px-4 py-3 text-sm font-medium border-b-2 transition-colors ${
						activeTab === "modules" ? "border-primary-500 text-primary-600" : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
					}`}
				>
					📦 Modules ({data.modules?.length || 0})
				</button>
				<button
					type="button"
					onClick={() => setActiveTab("resources")}
					className={`px-4 py-3 text-sm font-medium border-b-2 transition-colors ${
						activeTab === "resources" ? "border-primary-500 text-primary-600" : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
					}`}
				>
					🏗️ Resources ({data.resources?.length || 0})
				</button>
				<button
					type="button"
					onClick={() => setActiveTab("providers")}
					className={`px-4 py-3 text-sm font-medium border-b-2 transition-colors ${
						activeTab === "providers" ? "border-primary-500 text-primary-600" : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
					}`}
				>
					🔌 Providers ({data.providers?.length || 0})
				</button>
			</div>

			{/* Tab content */}
			<div className="flex-1 overflow-y-auto p-4">
				{activeTab === "modules" && (
					<div className="space-y-4">
						{data.modules?.map((module, index) => <ModuleCard key={module.path || index} module={module} onResourceSelect={onResourceSelect} />) || (
							<div className="text-center py-8 text-gray-500">
								<div className="text-4xl mb-2">📦</div>
								<p>No modules found</p>
							</div>
						)}
					</div>
				)}

				{activeTab === "resources" && (
					<div className="grid gap-4">
						{data.resources?.map((resource) => <ResourceCard key={resource.id} resource={resource} onResourceSelect={onResourceSelect} />) || (
							<div className="text-center py-8 text-gray-500">
								<div className="text-4xl mb-2">🏗️</div>
								<p>No resources found</p>
							</div>
						)}
					</div>
				)}

				{activeTab === "providers" && (
					<div className="space-y-4">
						{data.providers?.map((provider, index) => (
							<div key={index} className="bg-white border border-gray-200 rounded-lg p-4">
								<div className="flex items-center gap-3">
									<span className="text-xl">🔌</span>
									<div>
										<h3 className="font-semibold text-gray-900">{provider.name}</h3>
										{provider.alias && <p className="text-sm text-gray-600">Alias: {provider.alias}</p>}
									</div>
								</div>
							</div>
						)) || (
							<div className="text-center py-8 text-gray-500">
								<div className="text-4xl mb-2">🔌</div>
								<p>No providers found</p>
							</div>
						)}
					</div>
				)}
			</div>
		</div>
	);
}
