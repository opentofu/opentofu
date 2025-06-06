from setuptools import setup, find_packages

with open("README.md", "r", encoding="utf-8") as fh:
    long_description = fh.read()

setup(
    name="opentofu-middleware",
    version="0.1.0",
    author="OpenTofu Community",
    description="Python SDK for building OpenTofu middleware",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="https://github.com/opentofu/opentofu",
    package_dir={"": "src"},
    packages=find_packages(where="src"),
    classifiers=[
        "Development Status :: 3 - Alpha",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: Mozilla Public License 2.0 (MPL 2.0)",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
    ],
    python_requires=">=3.8",
    install_requires=[
        "jsonrpcserver>=5.0.0",
        "typing-extensions>=4.0.0",
    ],
    extras_require={
        "dev": [
            "black>=23.0.0",
            "pytest>=7.0.0",
            "pytest-asyncio>=0.21.0",
            "mypy>=1.0.0",
        ],
    },
)