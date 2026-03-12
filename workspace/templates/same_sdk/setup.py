from setuptools import setup, find_packages

setup(
    name="same-sdk",
    version="0.1.0",
    description="Python SDK for SAME (Stateless Agent Memory Engine)",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    author="Thirty3 Labs",
    author_email="dev@thirty3labs.com",
    url="https://statelessagent.com",
    packages=find_packages(),
    py_modules=["same"],
    python_requires=">=3.10",
    classifiers=[
        "Development Status :: 3 - Alpha",
        "Intended Audience :: Developers",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Programming Language :: Python :: 3.13",
        "Topic :: Software Development :: Libraries",
    ],
)
