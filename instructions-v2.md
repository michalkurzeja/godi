# Goal

Provide a comprehensive and detailed description of this library, including its purpose, features, internal behaviour, and how to use it effectively.
This description is aimed as a reference for AI agents with special purpose to compile and upgrade guide from older version to the current.

Be as detailed as possible.
Cover all aspects of the library, including its architecture, key components, and any relevant implementation details.
Include examples of how to use the library in different scenarios, and explain any important concepts or terminology that users should be familiar with.

# Results

Save the result of your work in a file: `docs/v2/documentation.md`.
It should consist of the following sections:
- Overview: A high-level summary of the library, its purpose, and its main features.
- Architecture: A detailed description of the library's architecture, including its key components and how they interact with each other.
- Source analysis: An analysis of EACH AND EVERY source file in the repository, describing its content and important implementation details. Clearly mark each file as a sub-section with its name as the title (in case of files in sub-directories, the section name is the local path).
- Additional remarks: Any additional information that may be relevant for understanding the library, such as known issues, limitations, or discrepancies observed during this research.

# Methodology

To achieve this goal, you should follow these steps:
- Familiarize yourself with the library by reading through the source code, tests and any existing documentation.
- For each Go source file in this repository:
	- Perform source analysis and write it to the documentation file under the appropriate section.
- Write the overview and architecture sections based on the information gathered from the source analysis and any additional research you may need to do.

# Additional remarks

Keep in mind that this documentation is intended primarily for AI agents.
It must be comprehensive, so that an AI agent can use it to understand the library in depth and use it effectively, even without further access to the source code.
The primary goal of this documentations is to use it to write a detailed upgrade guide from v1 to v2 of this library.
The upgrade guide will be given a similar documentation for v1 and v2 (which is the subject of this task).

The writing of v1 documentation and upgrade guide IS NOT PART OF THIS TASK!

You are NOT to change any files in the repository, except for the documentation file `docs/v2/documentation.md` where you will write the results of your work.

If you find any conflicting information between the source code and existing documentation, you should prioritize the information from the source code, as it is the most up-to-date and accurate representation of the library's functionality.
However, you should also note any discrepancies in the documentation file, as this may be useful for future reference and for improving the documentation in the future.